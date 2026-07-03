package postgres

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/piotrsenkow/mlsgrid-mcp/mls"
)

const (
	defaultCompsRadiusMiles = 1.0
	defaultClosedWithin     = 180 * 24 * time.Hour
	defaultCompsLimit       = 5
	maxCompsLimit           = 25
	compCandidateCap        = 200
)

// Balanced similarity weights (see docs/tools.md). Distance only participates
// when the subject has coordinates; otherwise its weight is renormalized away.
// Kept as named constants so the scoring is a one-line tuning change.
const (
	weightLivingArea = 0.40
	weightBeds       = 0.20
	weightBaths      = 0.15
	weightYear       = 0.15
	weightDistance   = 0.10
)

// subjectProfile is the normalized description of the property comps are drawn
// for, whether it came from an existing listing or an inline spec.
type subjectProfile struct {
	key          string // empty for an inline spec (no self-exclusion needed)
	area         mls.Area
	propertyType string
	propertySub  string
	livingArea   int64
	bedrooms     int
	bathsFull    int
	yearBuilt    int
	lat, lng     *float64
}

// FindComparables selects recent closed sales similar to a subject property and
// summarizes them into a suggested value range. It has no coordinates to lean on
// for many feeds (MRED omits them), so distance is used only when the subject
// carries lat/lng; otherwise comps are drawn from the subject's area and scored
// on attributes alone (DistanceMiles left nil).
func (a *Adapter) FindComparables(ctx context.Context, q mls.CompsQuery) (*mls.CompsResult, error) {
	subj, err := a.compSubject(ctx, q)
	if err != nil {
		return nil, err
	}

	radius := q.RadiusMiles
	if radius <= 0 {
		radius = defaultCompsRadiusMiles
	}
	within := q.ClosedWithin
	if within <= 0 {
		within = defaultClosedWithin
	}
	limit := q.Limit
	if limit <= 0 {
		limit = defaultCompsLimit
	}
	if limit > maxCompsLimit {
		limit = maxCompsLimit
	}

	candidates, err := a.compCandidates(ctx, subj, within, radius)
	if err != nil {
		return nil, err
	}

	geo := subj.lat != nil && subj.lng != nil
	comps := make([]mls.Comparable, 0, len(candidates))
	for _, c := range candidates {
		var dist *float64
		if geo && c.Latitude != nil && c.Longitude != nil {
			d := haversineMiles(*subj.lat, *subj.lng, *c.Latitude, *c.Longitude)
			if d > radius {
				continue // inside the bbox but outside the circle
			}
			dist = &d
		}
		sim, notes := scoreComp(subj, c, dist, radius)
		comps = append(comps, mls.Comparable{
			ListingSummary: c,
			DistanceMiles:  dist,
			Similarity:     sim,
			AdjustNotes:    notes,
		})
	}

	// Most similar first; closer breaks ties; then higher close price, then key.
	sort.SliceStable(comps, func(i, j int) bool {
		if comps[i].Similarity != comps[j].Similarity {
			return comps[i].Similarity > comps[j].Similarity
		}
		di, dj := distOrInf(comps[i].DistanceMiles), distOrInf(comps[j].DistanceMiles)
		if di != dj {
			return di < dj
		}
		if comps[i].ClosePrice != comps[j].ClosePrice {
			return comps[i].ClosePrice > comps[j].ClosePrice
		}
		return comps[i].ListingKey > comps[j].ListingKey
	})
	if len(comps) > limit {
		comps = comps[:limit]
	}

	result := &mls.CompsResult{Comparables: comps}
	valuation(subj, comps, result)
	if newest, ok, err := a.maxTimestamp(ctx, "property", "modification_timestamp"); err == nil {
		result.DataAsOf = orZeroTime(newest, ok)
	}
	return result, nil
}

// compSubject resolves the subject from a listing reference or an inline spec.
func (a *Adapter) compSubject(ctx context.Context, q mls.CompsQuery) (subjectProfile, error) {
	switch {
	case !q.Subject.Empty():
		d, err := a.GetListing(ctx, q.Subject, mls.ListingOptions{})
		if err != nil {
			return subjectProfile{}, err
		}
		return subjectProfile{
			key:          d.ListingKey,
			area:         mls.Area{City: d.Address.City, PostalCode: d.Address.PostalCode, County: d.Address.County, State: d.Address.State},
			propertyType: d.PropertyType,
			propertySub:  d.PropertySubType,
			livingArea:   d.LivingArea,
			bedrooms:     d.Bedrooms,
			bathsFull:    d.BathroomsFull,
			yearBuilt:    d.YearBuilt,
			lat:          d.Latitude,
			lng:          d.Longitude,
		}, nil
	case q.Spec != nil:
		s := q.Spec
		return subjectProfile{
			area:         s.Area,
			propertyType: s.PropertyType,
			livingArea:   s.LivingArea,
			bedrooms:     s.Bedrooms,
			bathsFull:    s.BathroomsFull,
			yearBuilt:    s.YearBuilt,
			lat:          s.Latitude,
			lng:          s.Longitude,
		}, nil
	default:
		return subjectProfile{}, fmt.Errorf("comps: provide a subject listing or an inline spec")
	}
}

// compCandidates returns the closed-sale pool for a subject: same product type,
// closed within the window, priced and sized (so PPSF is computable), scoped by
// coordinates (bbox) when available or by area otherwise.
func (a *Adapter) compCandidates(ctx context.Context, subj subjectProfile, within time.Duration, radius float64) ([]mls.ListingSummary, error) {
	var args argList
	conds := []string{
		"standard_status = 'Closed'",
		"close_price IS NOT NULL AND close_price > 0",
		"living_area IS NOT NULL AND living_area > 0",
	}
	cutoff := now().Add(-within).Format("2006-01-02")
	conds = append(conds, "close_date >= "+args.add(cutoff)+"::date")

	if subj.key != "" {
		conds = append(conds, "listing_key <> "+args.add(subj.key))
	}
	// Same product type: prefer the specific subtype, else the coarse type.
	if subj.propertySub != "" {
		conds = append(conds, "property_sub_type = "+args.add(subj.propertySub))
	} else if subj.propertyType != "" {
		conds = append(conds, "property_type = "+args.add(subj.propertyType))
	}

	if subj.lat != nil && subj.lng != nil {
		// Bounding-box prefilter around the subject; the haversine refine drops
		// the box corners. ~69 miles per degree of latitude.
		dLat := radius / 69.0
		dLng := radius / (69.0 * math.Cos(*subj.lat*math.Pi/180))
		if dLng < 0 {
			dLng = -dLng
		}
		conds = append(conds,
			fmt.Sprintf("latitude BETWEEN %s AND %s", args.add(*subj.lat-dLat), args.add(*subj.lat+dLat)),
			fmt.Sprintf("longitude BETWEEN %s AND %s", args.add(*subj.lng-dLng), args.add(*subj.lng+dLng)),
		)
	} else if sc := areaScopeOf(subj.area); sc.column != "" {
		if sc.caseInsensitive {
			conds = append(conds, fmt.Sprintf("lower(%s) = lower(%s)", sc.column, args.add(sc.value)))
		} else {
			conds = append(conds, sc.column+" = "+args.add(sc.value))
		}
	}

	sql := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY close_date DESC NULLS LAST LIMIT %d",
		summaryColumns, a.rel("property"), strings.Join(conds, " AND "), compCandidateCap)
	rows, err := a.pool.Query(ctx, sql, args.args...)
	if err != nil {
		return nil, fmt.Errorf("comps: candidates: %w", err)
	}
	defer rows.Close()

	var out []mls.ListingSummary
	for rows.Next() {
		s, err := scanSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("comps: scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// areaScope names a single column comparison for area-scoped candidate selection.
type areaScope struct {
	column          string
	value           string
	caseInsensitive bool
}

// areaScopeOf picks the tightest available area column, preferring locality names
// people actually search by (city) before falling back to county/postal/state.
func areaScopeOf(area mls.Area) areaScope {
	switch {
	case strings.TrimSpace(area.City) != "":
		return areaScope{"city", area.City, true}
	case strings.TrimSpace(area.County) != "":
		return areaScope{"county_or_parish", area.County, true}
	case strings.TrimSpace(area.PostalCode) != "":
		return areaScope{"postal_code", area.PostalCode, false}
	case strings.TrimSpace(area.State) != "":
		return areaScope{"state_or_province", area.State, true}
	default:
		return areaScope{}
	}
}

// scoreComp returns a 0..1 similarity plus human-readable adjustment notes. Only
// dimensions the subject actually specifies contribute; the used weights are
// renormalized, so a spec with just size+beds still scores sensibly.
func scoreComp(subj subjectProfile, c mls.ListingSummary, dist *float64, radius float64) (float64, []string) {
	var wsum, ssum float64
	var notes []string
	add := func(w, sub float64) {
		wsum += w
		ssum += w * sub
	}

	if subj.livingArea > 0 && c.LivingArea > 0 {
		diff := math.Abs(float64(c.LivingArea - subj.livingArea))
		add(weightLivingArea, 1-math.Min(1, diff/float64(subj.livingArea)))
		notes = append(notes, sqftNote(c.LivingArea-subj.livingArea))
	}
	if subj.bedrooms > 0 {
		diff := math.Abs(float64(c.Bedrooms - subj.bedrooms))
		add(weightBeds, 1-math.Min(1, diff/3))
		notes = append(notes, bedNote(c.Bedrooms-subj.bedrooms))
	}
	if subj.bathsFull > 0 {
		diff := math.Abs(float64(c.BathroomsFull - subj.bathsFull))
		add(weightBaths, 1-math.Min(1, diff/3))
	}
	if subj.yearBuilt > 0 && c.YearBuilt > 0 {
		diff := math.Abs(float64(c.YearBuilt - subj.yearBuilt))
		add(weightYear, 1-math.Min(1, diff/40))
		notes = append(notes, yearNote(c.YearBuilt-subj.yearBuilt))
	}
	if dist != nil {
		add(weightDistance, 1-math.Min(1, *dist/radius))
		notes = append(notes, fmt.Sprintf("%.1f mi away", *dist))
	}

	if wsum == 0 {
		return 0, notes
	}
	return ssum / wsum, notes
}

// valuation fills the summary metrics from the selected comps: median close
// price and $/sqft, and a suggested range from the interquartile $/sqft applied
// to the subject's size (falling back to interquartile close price when the
// subject's size is unknown).
func valuation(subj subjectProfile, comps []mls.Comparable, result *mls.CompsResult) {
	if len(comps) == 0 {
		return
	}
	closes := make([]float64, 0, len(comps))
	ppsf := make([]float64, 0, len(comps))
	for _, c := range comps {
		closes = append(closes, float64(c.ClosePrice))
		if c.LivingArea > 0 {
			ppsf = append(ppsf, float64(c.ClosePrice)/float64(c.LivingArea))
		}
	}
	sort.Float64s(closes)
	sort.Float64s(ppsf)

	result.MedianClosePrice = roundInt64(percentile(closes, 0.5))
	if len(ppsf) > 0 {
		result.MedianPPSF = roundInt64(percentile(ppsf, 0.5))
	}
	if subj.livingArea > 0 && len(ppsf) > 0 {
		result.SuggestedLow = roundInt64(percentile(ppsf, 0.25) * float64(subj.livingArea))
		result.SuggestedHigh = roundInt64(percentile(ppsf, 0.75) * float64(subj.livingArea))
	} else {
		result.SuggestedLow = roundInt64(percentile(closes, 0.25))
		result.SuggestedHigh = roundInt64(percentile(closes, 0.75))
	}
}

// percentile returns the linearly-interpolated p-quantile (0..1) of a slice that
// is already sorted ascending.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	rank := p * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	frac := rank - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// haversineMiles is the great-circle distance between two lat/lng points.
func haversineMiles(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMiles = 3958.7613
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusMiles * 2 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}

func distOrInf(d *float64) float64 {
	if d == nil {
		return math.Inf(1)
	}
	return *d
}

func roundInt64(f float64) int64 { return int64(math.Round(f)) }

func sqftNote(delta int64) string {
	switch {
	case delta == 0:
		return "same size"
	case delta > 0:
		return fmt.Sprintf("%d sqft larger", delta)
	default:
		return fmt.Sprintf("%d sqft smaller", -delta)
	}
}

func bedNote(delta int) string {
	switch {
	case delta == 0:
		return "same beds"
	case delta == 1:
		return "1 more bed"
	case delta == -1:
		return "1 fewer bed"
	case delta > 0:
		return fmt.Sprintf("%d more beds", delta)
	default:
		return fmt.Sprintf("%d fewer beds", -delta)
	}
}

func yearNote(delta int) string {
	switch {
	case delta == 0:
		return "same era"
	case delta > 0:
		return fmt.Sprintf("%d yrs newer", delta)
	default:
		return fmt.Sprintf("%d yrs older", -delta)
	}
}

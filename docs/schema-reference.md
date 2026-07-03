# Schema reference

The database mlsgrid-mcp reads is owned by
[mlsgrid-sync](https://github.com/piotrsenkow/mlsgrid-sync) (its
`docs/schema-contract.md`). This page is a human companion to the
**`describe_dataset`** tool, which returns the same information *live* — column
names, types, and the actual distinct values of categorical columns. When in
doubt, trust `describe_dataset` (or a `SELECT DISTINCT`): values are data-driven
and this page is a snapshot.

Column names are **snake_case**. Categorical values are **case-sensitive** in
raw SQL — the curated tools (`search_listings`, `market_stats`) match them
case-insensitively, but `query_sql` does not.

## Gotchas that silently return zero rows

- **Two status columns.** `standard_status` is the RESO-canonical value the
  curated tools filter on; `mls_status` is the MLS's own, more granular value.
  They can spell the same concept differently — e.g. `standard_status` uses
  `Canceled` (one L) while `mls_status` uses `Cancelled` (two L).
- **`mls_status`-only concepts.** `New`, `Contingent`, `Price Change`, `Rented`,
  `Re-activated` exist only in `mls_status`. In `standard_status`, "new" listings
  are `Active` and "contingent" is `Active Under Contract`.
- **`property_sub_type` is sparse** — frequently NULL, and (in MRED) populated
  mainly for commercial property. Do not filter residential by sub-type; use
  `property_type`.
- **No coordinates.** `latitude`/`longitude` exist but MRED leaves them NULL, so
  distance-based comps fall back to area matching.
- **Money and areas are `numeric`**, so `query_sql` returns them as strings
  (to preserve precision).

## Categorical values (observed)

| Column | Values |
|---|---|
| `property.standard_status` | `Active`, `Closed`, `Active Under Contract`, `Pending`, `Canceled`, `Expired`, `Hold` |
| `property.mls_status` | `Active`, `New`, `Closed`, `Contingent`, `Active (Private)`, `Cancelled`, `Expired`, `Pending`, `Price Change`, `Rented`, `Re-activated`, `Temporarily No Showings`, `Contingent (Private)`, `Cancelled (Private)`, `Expired (Private)`, `Pending (Private)`, `Auction`, `Comparable Only listing - Closed` |
| `property.property_type` | `Residential`, `Residential Lease`, `Residential Income`, `Land`, `Commercial Sale`, `Commercial Lease`, `Manufactured In Park`, `Farm`, `Business Opportunity` |
| `property.property_sub_type` | sparse; e.g. `Mixed Use`, `Office`, `Retail`, `Warehouse`, `Restaurant`, `Condominium`, `Apartment`, `Shopping Center`, `Business`, `Other` |
| `property.association_fee_frequency` | `Not Applicable`, `Monthly`, `Annually`, `Quarterly`, `Voluntary` |
| `listing_event.event_type` | `new_listing`, `price_change`, `status_change`, `back_on_market`, `delisted` |
| `media.storage_status` | `skipped`, `pending`, `downloaded`, `failed` |

## Tables

`property` is the main table. The others link back to it by `listing_key`.

### `property` (columns by group)

- **Identity:** `listing_key`, `listing_id`, `originating_system_name`
- **Status/type:** `standard_status`, `mls_status`, `property_type`, `property_sub_type`
- **Price:** `list_price`, `original_list_price`, `previous_list_price`, `close_price` (all `numeric`)
- **Dates:** `listing_contract_date`, `purchase_contract_date`, `close_date`, `off_market_date`, `days_on_market`, `cumulative_days_on_market`, `modification_timestamp`, `original_entry_timestamp`, `status_change_timestamp`, `photos_change_timestamp`
- **Location:** `street_number`, `street_dir_prefix`, `street_name`, `street_suffix`, `unit_number`, `city`, `postal_code`, `county_or_parish`, `state_or_province`, `township`, `subdivision_name`, `latitude`, `longitude`
- **Size/rooms:** `bedrooms_total`, `bathrooms_full`, `bathrooms_half`, `rooms_total`, `living_area`, `building_area_total`, `lot_size_acres`, `lot_size_square_feet`, `year_built`, `stories_total`, `garage_spaces`, `parking_total`, `number_of_units_total`
- **Flags (bool):** `new_construction_yn`, `property_attached_yn`, `waterfront_yn`, `internet_address_display_yn`, `internet_entire_listing_display_yn`
- **Description:** `public_remarks`, `virtual_tour_url`
- **Schools:** `elementary_school`, `middle_or_junior_school`, `high_school` (+ `_district` each)
- **Agents/offices:** `list_agent_full_name`, `list_agent_mls_id`, `list_agent_key`, `list_office_name`, `list_office_mls_id`, `buyer_agent_full_name`, `buyer_agent_mls_id`, `buyer_office_name`
- **Tax/HOA:** `tax_annual_amount`, `tax_year`, `association_fee`, `association_fee_frequency`, `parcel_number`
- **Media/misc:** `photos_count`, `raw` (jsonb)

### Related tables

- **`listing_event`** — `listing_key`, `event_type`, `old_value`, `new_value`, `observed_at` (the change timeline behind `price_history`).
- **`media`** — `media_key`, `listing_key`, `media_url`, `display_order`, `caption`, `storage_status`, `local_path`, `content_type`, `bytes`.
- **`open_house`** — `open_house_key`, `listing_key`, `open_house_date`, `start_time`, `end_time`, `remarks`.
- **`room`** — `listing_key`, `room_type`, `room_level`, `room_dimensions`.
- **`unit_type`** — `listing_key`, `unit_number`, `beds_total`, `baths_total`, `actual_rent` (for multi-unit / income property).

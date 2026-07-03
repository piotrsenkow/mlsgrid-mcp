-- Deterministic fixture for the B-M4 market_stats / get_open_houses integration
-- tests. Applied on top of the pinned contract migration
-- (testdata/contract/0001_init.sql) into a *separate* throwaway schema
-- (mlsgrid_market) so it never perturbs the main seed's row-count assertions.
--
-- The market tests pin the adapter clock to 2026-07-01 with a 90-day default
-- window, which splits the closings into:
--   current window [2026-04-02, 2026-07-01]  → the RVT-C* rows
--   prior window   [2026-01-02, 2026-04-02)  → the RVT-P* rows
-- Numbers are hand-tuned so medians land on round values:
--   current closes 400/450/500/550/600k → median 500000, avg 500000
--   current $/sqft 200/200/200/220/200   → median 200
--   current DOM    20/30/40/50/60        → median 40
--   current CDOM   20/45/40/80/60        → median 45
-- Four active RVT listings (list 420/480/520/560k → median 500000) give an
-- inventory of 4, so months of supply = 4 ÷ (5 sales / 3 months) = 2.4.
--
-- The ELS-* rows live in a different city ("Elsewhere") with extreme prices, so
-- any test whose area filter regressed to matching everything would visibly
-- skew. All open houses hang off RVT active listings except OH-ELS-1.

INSERT INTO property (
    listing_key, listing_id, originating_system_name, standard_status,
    property_type, property_sub_type, list_price, original_list_price, close_price,
    close_date, bedrooms_total, bathrooms_full, living_area, year_built,
    days_on_market, cumulative_days_on_market, city, postal_code, county_or_parish,
    state_or_province, modification_timestamp
) VALUES
-- Current-window closed sales (Rivertown, Residential SFR).
('RVT-C1', 'C1', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    410000, 430000, 400000, '2026-05-01', 3, 2, 2000, 1998, 20, 20,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-05-02T09:00:00Z'),
('RVT-C2', 'C2', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    460000, 470000, 450000, '2026-05-15', 3, 2, 2250, 2003, 30, 45,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-05-16T09:00:00Z'),
('RVT-C3', 'C3', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    500000, 520000, 500000, '2026-06-01', 4, 3, 2500, 2008, 40, 40,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-02T09:00:00Z'),
('RVT-C4', 'C4', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    560000, 560000, 550000, '2026-06-10', 4, 3, 2500, 2012, 50, 80,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-11T09:00:00Z'),
('RVT-C5', 'C5', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    620000, 640000, 600000, '2026-06-20', 5, 4, 3000, 2016, 60, 60,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-21T09:00:00Z'),
-- Prior-window closed sales (Rivertown, Residential SFR).
('RVT-P1', 'P1', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    400000, 400000, 380000, '2026-02-01', 3, 2, 2000, 1995, 25, 25,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-02-02T09:00:00Z'),
('RVT-P2', 'P2', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    435000, 450000, 420000, '2026-02-20', 3, 2, 2100, 2000, 35, 35,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-02-21T09:00:00Z'),
('RVT-P3', 'P3', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    470000, 480000, 460000, '2026-03-15', 4, 3, 2300, 2005, 45, 45,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-03-16T09:00:00Z'),
-- Active inventory (Rivertown, Residential SFR).
('RVT-A1', 'A1', 'mred', 'Active', 'Residential', 'Single Family Residence',
    420000, 420000, NULL, NULL, 3, 2, 2050, 1999, 12, 12,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-25T09:00:00Z'),
('RVT-A2', 'A2', 'mred', 'Active', 'Residential', 'Single Family Residence',
    480000, 480000, NULL, NULL, 3, 2, 2200, 2004, 8, 8,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-26T09:00:00Z'),
('RVT-A3', 'A3', 'mred', 'Active', 'Residential', 'Single Family Residence',
    520000, 530000, NULL, NULL, 4, 3, 2400, 2010, 22, 22,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-27T09:00:00Z'),
('RVT-A4', 'A4', 'mred', 'Active', 'Residential', 'Single Family Residence',
    560000, 575000, NULL, NULL, 4, 3, 2600, 2014, 30, 30,
    'Rivertown', '60000', 'Riverside', 'IL', '2026-06-30T09:00:00Z'),
-- Decoys in another city with extreme prices: a working area filter excludes
-- these; a broken one would swing every median.
('ELS-C1', 'EC1', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    1000000, 1000000, 990000, '2026-06-05', 6, 5, 5000, 2020, 100, 100,
    'Elsewhere', '70000', 'Farland', 'IL', '2026-06-06T09:00:00Z'),
('ELS-A1', 'EA1', 'mred', 'Active', 'Residential', 'Single Family Residence',
    900000, 900000, NULL, NULL, 5, 4, 4000, 2018, 5, 5,
    'Elsewhere', '70000', 'Farland', 'IL', '2026-06-28T09:00:00Z');

-- Open houses: three on Rivertown active listings, one Elsewhere. start/end are
-- full timestamps; open_house_date is the calendar day the tests window on.
INSERT INTO open_house (
    open_house_key, listing_key, listing_id, open_house_date,
    start_time, end_time, remarks, modification_timestamp
) VALUES
('OH-A1-1', 'RVT-A1', 'A1', '2026-07-04',
    '2026-07-04T16:00:00Z', '2026-07-04T18:00:00Z', 'Independence weekend open house', '2026-07-01T09:00:00Z'),
('OH-A1-2', 'RVT-A1', 'A1', '2026-07-11',
    '2026-07-11T13:00:00Z', '2026-07-11T15:00:00Z', 'Second showing', '2026-07-06T09:00:00Z'),
('OH-A2-1', 'RVT-A2', 'A2', '2026-07-05',
    '2026-07-05T12:00:00Z', '2026-07-05T14:00:00Z', 'Sunday open house', '2026-07-01T09:00:00Z'),
('OH-ELS-1', 'ELS-A1', 'EA1', '2026-07-04',
    '2026-07-04T11:00:00Z', '2026-07-04T13:00:00Z', 'Elsewhere open house', '2026-07-01T09:00:00Z');

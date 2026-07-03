-- Deterministic fixture for the B-M2 query-core integration tests. Applied on
-- top of the pinned contract migration (testdata/contract/0001_init.sql) into a
-- throwaway schema. Table names are unqualified: the harness sets search_path to
-- the test schema before running this, exactly as mlsgrid-sync's migrator does.
--
-- The 12 property rows are hand-tuned so the tests can assert exact counts and a
-- stable keyset ordering. Two rows share a modification_timestamp (MRD1001 /
-- MRD1007) to exercise the (modification_timestamp, listing_key) cursor
-- tiebreak, and two rows share listing_id '9999' across two originating systems
-- (MRD2001 / CML3001) to exercise get_listing's cross-feed ambiguity handling.
--
-- Keyset order (modification_timestamp DESC, listing_key DESC):
--   MRD1008, MRD1004, CML3001, MRD2001, MRD1002, MRD1007, MRD1001,
--   MRD1010, MRD1003, MRD1005, MRD1006, MRD1009

INSERT INTO property (
    listing_key, listing_id, originating_system_name, standard_status,
    property_type, property_sub_type, list_price, close_price,
    bedrooms_total, bathrooms_full, bathrooms_half, living_area, year_built,
    days_on_market, street_number, street_name, street_suffix, city,
    postal_code, county_or_parish, state_or_province, latitude, longitude,
    public_remarks, modification_timestamp
) VALUES
('MRD1001', '1001', 'mred', 'Active', 'Residential', 'Single Family Residence',
    350000, NULL, 3, 2, 1, 1800, 1995, 10, '934', 'Wolcott', 'Ave', 'Chicago',
    '60601', 'Cook', 'IL', 41.8800, -87.6200,
    'Sunny bungalow near the park', '2026-06-01T09:00:00Z'),
('MRD1002', '1002', 'mred', 'Active', 'Residential', 'Condominium',
    250000, NULL, 2, 1, 0, 1100, 2005, 45, '2100', 'Lincoln', 'Ave', 'Chicago',
    '60614', 'Cook', 'IL', NULL, NULL,
    'Renovated condo with lake views', '2026-06-05T09:00:00Z'),
('MRD1004', '1004', 'mred', 'Pending', 'Residential', 'Townhouse',
    320000, NULL, 3, 2, 1, 1600, 2015, 15, '55', 'Water', 'St', 'Naperville',
    '60540', 'DuPage', 'IL', NULL, NULL,
    'Modern townhome close to the Metra', '2026-06-10T09:00:00Z'),
('MRD1005', '1005', 'mred', 'Active', 'Residential Income', 'Multi Family',
    675000, NULL, 6, 4, 0, 3200, 1920, 90, '1600', 'Damen', 'Ave', 'Chicago',
    '60622', 'Cook', 'IL', 41.9000, -87.6800,
    'Two-flat investment with long-term tenants', '2026-04-15T09:00:00Z'),
('MRD1006', '1006', 'mred', 'Active', 'Residential', 'Single Family Residence',
    189000, NULL, 2, 1, 0, 950, 1955, 120, '300', 'Oak Park', 'Ave', 'Oak Park',
    '60302', 'Cook', 'IL', NULL, NULL,
    'Starter home that needs some work', '2026-03-30T09:00:00Z'),
('MRD1007', '1007', 'mred', 'Closed', 'Residential', 'Condominium',
    425000, 410000, 3, 2, 0, 1500, 2018, 30, '401', 'Wabash', 'Ave', 'Chicago',
    '60601', 'Cook', 'IL', 41.8850, -87.6220,
    'High-floor condo with a city skyline view', '2026-06-01T09:00:00Z'),
('MRD1008', '1008', 'mred', 'Active', 'Residential', 'Single Family Residence',
    1200000, NULL, 5, 4, 1, 4200, 2020, 5, '99', 'Prairie', 'Ln', 'Naperville',
    '60565', 'DuPage', 'IL', NULL, NULL,
    'Luxury new construction with a pool', '2026-06-12T09:00:00Z'),
('MRD1009', '1009', 'mred', 'Active', 'Residential', 'Single Family Residence',
    275000, NULL, 3, 1, 1, 1400, 1970, 200, '5200', 'Kedzie', 'Ave', 'Chicago',
    '60629', 'Cook', 'IL', NULL, NULL,
    'Brick ranch on a large lot', '2026-02-01T09:00:00Z'),
('MRD1010', '1010', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    620000, 600000, 4, 3, 0, 2900, 2001, 40, '1200', 'Sheridan', 'Rd', 'Evanston',
    '60202', 'Cook', 'IL', 42.0500, -87.6800,
    'Lakeside retreat with a garden', '2026-05-25T09:00:00Z'),
('MRD2001', '9999', 'mred', 'Active', 'Residential', 'Condominium',
    300000, NULL, 2, 2, 0, 1200, 2010, 20, '740', 'Federal', 'St', 'Chicago',
    '60605', 'Cook', 'IL', NULL, NULL,
    'Loft in the South Loop', '2026-06-08T09:00:00Z'),
('CML3001', '9999', 'connectmls', 'Active', 'Residential', 'Condominium',
    310000, NULL, 2, 2, 0, 1250, 2011, 22, '740', 'Federal', 'St', 'Chicago',
    '60605', 'Cook', 'IL', NULL, NULL,
    'Loft in the South Loop, connectMLS feed', '2026-06-09T09:00:00Z');

-- MRD1003 is the fully-populated detail row used by get_listing assertions.
INSERT INTO property (
    listing_key, listing_id, originating_system_name, standard_status,
    property_type, property_sub_type, list_price, original_list_price,
    close_price, bedrooms_total, bathrooms_full, bathrooms_half, living_area,
    lot_size_acres, year_built, days_on_market, street_number, street_dir_prefix,
    street_name, street_suffix, unit_number, city, postal_code, county_or_parish,
    state_or_province, latitude, longitude, public_remarks, virtual_tour_url,
    association_fee, tax_annual_amount, tax_year, list_agent_full_name,
    list_office_name, photos_count, mlg_can_use, raw, modification_timestamp
) VALUES (
    'MRD1003', '1003', 'mred', 'Closed', 'Residential', 'Single Family Residence',
    500000, 520000, 485000, 4, 3, 1, 2600, 0.25, 1988, 60, '1500', 'N',
    'Ridge', 'Ave', NULL, 'Evanston', '60201', 'Cook', 'IL', 42.0400, -87.6900,
    'Spacious colonial with an updated kitchen', 'https://tours.example.com/1003',
    0, 8200, 2025, 'Jane Broker', 'North Shore Realty', 25, '{IDX}',
    '{"MRD_extra":"legacy-field","GrossIncome":42000}'::jsonb,
    '2026-05-20T09:00:00Z');

-- A little child + media data so freshness/detail reads exercise real joins.
INSERT INTO room (listing_key, room_key, room_type, room_level, room_dimensions) VALUES
('MRD1003', 'MRD1003-R1', 'Living Room', 'Main', '20x15'),
('MRD1003', 'MRD1003-R2', 'Primary Bedroom', 'Second', '16x14');

INSERT INTO media (media_key, listing_key, media_url, display_order, storage_status) VALUES
('MRD1003-M1', 'MRD1003', 'https://media.example.com/1003/1.jpg', 1, 'skipped'),
('MRD1003-M2', 'MRD1003', 'https://media.example.com/1003/2.jpg', 2, 'skipped'),
('MRD1001-M1', 'MRD1001', 'https://media.example.com/1001/1.jpg', 1, 'skipped');

-- Replication cursors: a completed backfill per originating system so freshness
-- reads a coherent picture.
INSERT INTO sync_state (resource, originating_system, last_modification_ts, backfill_completed_at, updated_at) VALUES
('Property', 'mred', '2026-06-12T09:00:00Z', '2026-05-01T00:00:00Z', now()),
('Property', 'connectmls', '2026-06-09T09:00:00Z', '2026-05-01T00:00:00Z', now());

-- One change-capture event so Capabilities reports price history available.
INSERT INTO listing_event (listing_key, event_type, old_value, new_value, observed_at, source_modification_timestamp) VALUES
('MRD1003', 'price_change', '520000', '500000', '2026-05-10T09:00:00Z', '2026-05-10T09:00:00Z'),
('MRD1003', 'status_change', 'Active', 'Closed', '2026-05-20T09:00:00Z', '2026-05-20T09:00:00Z');

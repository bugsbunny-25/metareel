-- +goose Up
-- HDR by Barco: Barco's high-dynamic-range laser projection in premium
-- cinema screens. Its logo is uploaded by an admin like the others.
INSERT INTO watch_formats (slug, label, category, sort_order) VALUES
    ('hdr-by-barco', 'HDR by Barco', 'projection', 74)
ON CONFLICT (slug) DO NOTHING;

-- +goose Down
DELETE FROM watch_formats WHERE slug = 'hdr-by-barco';

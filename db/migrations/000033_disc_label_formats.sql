-- +goose Up
-- Home media joins the format taxonomy as two more axes: the disc a film was
-- watched on (DVD, Blu-ray, 4K UHD Blu-ray) and the label that released it
-- (Criterion, Arrow, ...). As formats they get admin-managed logos and show
-- on watches like cinema formats do. Plus more common premium cinema
-- formats. Logos are uploaded by admins; none are seeded.
INSERT INTO watch_formats (slug, label, category, sort_order) VALUES
    ('rpx',          'Regal RPX',              'brand',     45),
    ('prime-at-amc', 'PRIME at AMC',           'brand',     46),
    ('superscreen',  'Cineworld Superscreen',  'brand',     47),
    ('isense',       'ODEON iSense',           'brand',     48),
    ('ultraavx',     'Cineplex UltraAVX',      'brand',     49),
    ('vmax',         'Event Cinemas Vmax',     'brand',     50),
    ('macro-xe',     'Cinépolis Macro XE',     'brand',     51),
    ('pxl',          'PVR P[XL]',              'brand',     52),
    ('mx4d',         'MX4D',                   'motion',    32),
    ('dolby-3d',     'Dolby 3D',               'dimension', 51),
    ('thx',          'THX',                    'sound',     62),
    ('auro-3d',      'Auro 3D',                'sound',     63),

    ('uhd-blu-ray',  '4K UHD Blu-ray',         'disc',     100),
    ('blu-ray',      'Blu-ray',                'disc',     101),
    ('dvd',          'DVD',                    'disc',     102),

    ('criterion',        'The Criterion Collection', 'label', 200),
    ('arrow-video',      'Arrow Video',              'label', 201),
    ('kino-lorber',      'Kino Lorber',              'label', 202),
    ('eureka',           'Eureka (Masters of Cinema)', 'label', 203),
    ('bfi',              'BFI',                      'label', 204),
    ('second-sight',     'Second Sight',             'label', 205),
    ('88-films',         '88 Films',                 'label', 206),
    ('vinegar-syndrome', 'Vinegar Syndrome',         'label', 207),
    ('severin',          'Severin Films',            'label', 208),
    ('shout-factory',    'Shout! Factory',           'label', 209),
    ('radiance',         'Radiance Films',           'label', 210),
    ('indicator',        'Indicator',                'label', 211),
    ('umbrella',         'Umbrella Entertainment',   'label', 212),
    ('imprint',          'Imprint Films',            'label', 213),
    ('studiocanal',      'StudioCanal',              'label', 214)
ON CONFLICT (slug) DO NOTHING;

-- +goose Down
DELETE FROM watch_formats WHERE slug IN (
    'rpx', 'prime-at-amc', 'superscreen', 'isense', 'ultraavx', 'vmax', 'macro-xe', 'pxl',
    'mx4d', 'dolby-3d', 'thx', 'auro-3d',
    'uhd-blu-ray', 'blu-ray', 'dvd',
    'criterion', 'arrow-video', 'kino-lorber', 'eureka', 'bfi', 'second-sight', '88-films',
    'vinegar-syndrome', 'severin', 'shout-factory', 'radiance', 'indicator', 'umbrella',
    'imprint', 'studiocanal');

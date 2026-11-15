// The choices a watch records (where, screening, plan, media, picture and sound), as
// [value, label] pairs: shared by the log dialog and the watch page.

export const PLACES = [['', 'Not saying'], ['theater', 'Cinema'], ['streaming', 'Streaming'], ['physical', 'Physical media'], ['broadcast', 'TV broadcast'], ['other', 'Elsewhere']]
export const PRECISIONS = [['minute', 'Exact time'], ['day', 'Day'], ['month', 'Month'], ['year', 'Year']]
export const SCREENINGS = [['', 'Regular show'], ['preview', 'Preview or early access'], ['premiere', 'Premiere'], ['festival', 'Festival'], ['press', 'Press screening'], ['rerelease', 'Re-release'], ['marathon', 'Marathon']]
export const PLANS = [['flatrate', 'Subscription'], ['free', 'Free'], ['ads', 'Free with ads'], ['rent', 'Rented'], ['buy', 'Bought']]
// Discs come from the format taxonomy (with logos); these are the other media.
export const OTHER_MEDIA = [['', '–'], ['vhs', 'VHS'], ['digital_file', 'Digital file'], ['35mm', '35mm print'], ['70mm', '70mm print']]
export const DIMENSIONS = [['', '–'], ['2d', '2D'], ['3d', '3D'], ['3d_hfr', '3D HFR']]
export const RESOLUTIONS = [['', '–'], ['sd', 'SD'], ['hd', 'HD 720p'], ['fhd', 'Full HD 1080p'], ['4k', '4K'], ['8k', '8K']]
export const HDRS = [['', '–'], ['none', 'No HDR'], ['hdr10', 'HDR10'], ['hdr10_plus', 'HDR10+'], ['dolby_vision', 'Dolby Vision'], ['hlg', 'HLG']]
export const DEVICES = ['TV', 'Projector', 'Laptop', 'Tablet', 'Phone']
export const AUDIO = ['Dolby Atmos', 'DTS:X', 'Dolby TrueHD', '7.1', '5.1', 'Stereo']
export const ASPECTS = ['1.43:1', '1.66:1', '1.78:1', '1.85:1', '1.90:1', '2.00:1', '2.20:1', '2.39:1', '2.76:1', '4:3']

/** The label for a stored value, or the value itself when it isn't one of ours. */
export const labelOf = (list, v) => (v ? list.find(([k]) => k === v)?.[1] ?? v : '')

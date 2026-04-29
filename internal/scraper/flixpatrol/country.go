package flixpatrol

import (
	"fmt"
	"strings"

	"github.com/bugsbunny-25/metareel/internal/constants"
)

// slugToCode maps FlixPatrol URL slugs to ISO 3166-1 alpha-2 codes.
var slugToCode = map[string]constants.CountryCode{
	"afghanistan":                      constants.CountryCode_AF,
	"albania":                          constants.CountryCode_AL,
	"algeria":                          constants.CountryCode_DZ,
	"andorra":                          constants.CountryCode_AD,
	"angola":                           constants.CountryCode_AO,
	"antigua-and-barbuda":              constants.CountryCode_AG,
	"argentina":                        constants.CountryCode_AR,
	"armenia":                          constants.CountryCode_AM,
	"australia":                        constants.CountryCode_AU,
	"austria":                          constants.CountryCode_AT,
	"azerbaijan":                       constants.CountryCode_AZ,
	"bahamas":                          constants.CountryCode_BS,
	"bahrain":                          constants.CountryCode_BH,
	"bangladesh":                       constants.CountryCode_BD,
	"barbados":                         constants.CountryCode_BB,
	"belarus":                          constants.CountryCode_BY,
	"belgium":                          constants.CountryCode_BE,
	"belize":                           constants.CountryCode_BZ,
	"benin":                            constants.CountryCode_BJ,
	"bolivia":                          constants.CountryCode_BO,
	"bosnia-and-herzegovina":           constants.CountryCode_BA,
	"botswana":                         constants.CountryCode_BW,
	"brazil":                           constants.CountryCode_BR,
	"brunei":                           constants.CountryCode_BN,
	"bulgaria":                         constants.CountryCode_BG,
	"burkina-faso":                     constants.CountryCode_BF,
	"burundi":                          constants.CountryCode_BI,
	"cambodia":                         constants.CountryCode_KH,
	"cameroon":                         constants.CountryCode_CM,
	"canada":                           constants.CountryCode_CA,
	"cape-verde":                       constants.CountryCode_CV,
	"chad":                             constants.CountryCode_TD,
	"chile":                            constants.CountryCode_CL,
	"colombia":                         constants.CountryCode_CO,
	"costa-rica":                       constants.CountryCode_CR,
	"croatia":                          constants.CountryCode_HR,
	"cyprus":                           constants.CountryCode_CY,
	"czech-republic":                   constants.CountryCode_CZ,
	"democratic-republic-of-the-congo": constants.CountryCode_CD,
	"denmark":                          constants.CountryCode_DK,
	"dominica":                         constants.CountryCode_DM,
	"dominican-republic":               constants.CountryCode_DO,
	"ecuador":                          constants.CountryCode_EC,
	"egypt":                            constants.CountryCode_EG,
	"estonia":                          constants.CountryCode_EE,
	"fiji":                             constants.CountryCode_FJ,
	"finland":                          constants.CountryCode_FI,
	"france":                           constants.CountryCode_FR,
	"gabon":                            constants.CountryCode_GA,
	"gambia":                           constants.CountryCode_GM,
	"georgia":                          constants.CountryCode_GE,
	"germany":                          constants.CountryCode_DE,
	"ghana":                            constants.CountryCode_GH,
	"greece":                           constants.CountryCode_GR,
	"grenada":                          constants.CountryCode_GD,
	"guadeloupe":                       constants.CountryCode_GP,
	"guatemala":                        constants.CountryCode_GT,
	"guinea-bissau":                    constants.CountryCode_GW,
	"guyana":                           constants.CountryCode_GY,
	"haiti":                            constants.CountryCode_HT,
	"honduras":                         constants.CountryCode_HN,
	"hong-kong":                        constants.CountryCode_HK,
	"hungary":                          constants.CountryCode_HU,
	"iceland":                          constants.CountryCode_IS,
	"india":                            constants.CountryCode_IN,
	"indonesia":                        constants.CountryCode_ID,
	"iraq":                             constants.CountryCode_IQ,
	"ireland":                          constants.CountryCode_IE,
	"israel":                           constants.CountryCode_IL,
	"italy":                            constants.CountryCode_IT,
	"ivory-coast":                      constants.CountryCode_CI,
	"jamaica":                          constants.CountryCode_JM,
	"japan":                            constants.CountryCode_JP,
	"jordan":                           constants.CountryCode_JO,
	"kazakhstan":                       constants.CountryCode_KZ,
	"kenya":                            constants.CountryCode_KE,
	"kuwait":                           constants.CountryCode_KW,
	"kyrgyzstan":                       constants.CountryCode_KG,
	"laos":                             constants.CountryCode_LA,
	"latvia":                           constants.CountryCode_LV,
	"lebanon":                          constants.CountryCode_LB,
	"libya":                            constants.CountryCode_LY,
	"liechtenstein":                    constants.CountryCode_LI,
	"lithuania":                        constants.CountryCode_LT,
	"luxembourg":                       constants.CountryCode_LU,
	"madagascar":                       constants.CountryCode_MG,
	"malawi":                           constants.CountryCode_MW,
	"malaysia":                         constants.CountryCode_MY,
	"maldives":                         constants.CountryCode_MV,
	"mali":                             constants.CountryCode_ML,
	"malta":                            constants.CountryCode_MT,
	"martinique":                       constants.CountryCode_MQ,
	"mauritania":                       constants.CountryCode_MR,
	"mauritius":                        constants.CountryCode_MU,
	"mexico":                           constants.CountryCode_MX,
	"moldova":                          constants.CountryCode_MD,
	"monaco":                           constants.CountryCode_MC,
	"mongolia":                         constants.CountryCode_MN,
	"montenegro":                       constants.CountryCode_ME,
	"montserrat":                       constants.CountryCode_MS,
	"morocco":                          constants.CountryCode_MA,
	"mozambique":                       constants.CountryCode_MZ,
	"myanmar":                          constants.CountryCode_MM,
	"namibia":                          constants.CountryCode_NA,
	"netherlands":                      constants.CountryCode_NL,
	"new-caledonia":                    constants.CountryCode_NC,
	"new-zealand":                      constants.CountryCode_NZ,
	"nicaragua":                        constants.CountryCode_NI,
	"niger":                            constants.CountryCode_NE,
	"nigeria":                          constants.CountryCode_NG,
	"north-macedonia":                  constants.CountryCode_MK,
	"norway":                           constants.CountryCode_NO,
	"oman":                             constants.CountryCode_OM,
	"pakistan":                         constants.CountryCode_PK,
	"panama":                           constants.CountryCode_PA,
	"papua-new-guinea":                 constants.CountryCode_PG,
	"paraguay":                         constants.CountryCode_PY,
	"peru":                             constants.CountryCode_PE,
	"philippines":                      constants.CountryCode_PH,
	"poland":                           constants.CountryCode_PL,
	"portugal":                         constants.CountryCode_PT,
	"qatar":                            constants.CountryCode_QA,
	"republic-of-the-congo":            constants.CountryCode_CG,
	"reunion":                          constants.CountryCode_RE,
	"romania":                          constants.CountryCode_RO,
	"russia":                           constants.CountryCode_RU,
	"rwanda":                           constants.CountryCode_RW,
	"saint-kitts-and-nevis":            constants.CountryCode_KN,
	"saint-lucia":                      constants.CountryCode_LC,
	"salvador":                         constants.CountryCode_SV,
	"san-marino":                       constants.CountryCode_SM,
	"saudi-arabia":                     constants.CountryCode_SA,
	"senegal":                          constants.CountryCode_SN,
	"serbia":                           constants.CountryCode_RS,
	"singapore":                        constants.CountryCode_SG,
	"slovakia":                         constants.CountryCode_SK,
	"slovenia":                         constants.CountryCode_SI,
	"somalia":                          constants.CountryCode_SO,
	"south-africa":                     constants.CountryCode_ZA,
	"south-korea":                      constants.CountryCode_KR,
	"south-sudan":                      constants.CountryCode_SS,
	"spain":                            constants.CountryCode_ES,
	"sri-lanka":                        constants.CountryCode_LK,
	"sudan":                            constants.CountryCode_SD,
	"suriname":                         constants.CountryCode_SR,
	"swaziland":                        constants.CountryCode_SZ,
	"sweden":                           constants.CountryCode_SE,
	"switzerland":                      constants.CountryCode_CH,
	"taiwan":                           constants.CountryCode_TW,
	"tajikistan":                       constants.CountryCode_TJ,
	"tanzania":                         constants.CountryCode_TZ,
	"thailand":                         constants.CountryCode_TH,
	"togo":                             constants.CountryCode_TG,
	"trinidad-and-tobago":              constants.CountryCode_TT,
	"tunisia":                          constants.CountryCode_TN,
	"turkey":                           constants.CountryCode_TR,
	"turkmenistan":                     constants.CountryCode_TM,
	"uganda":                           constants.CountryCode_UG,
	"ukraine":                          constants.CountryCode_UA,
	"united-arab-emirates":             constants.CountryCode_AE,
	"united-kingdom":                   constants.CountryCode_GB,
	"united-states":                    constants.CountryCode_US,
	"uruguay":                          constants.CountryCode_UY,
	"uzbekistan":                       constants.CountryCode_UZ,
	"venezuela":                        constants.CountryCode_VE,
	"vietnam":                          constants.CountryCode_VN,
	"yemen":                            constants.CountryCode_YE,
	"zambia":                           constants.CountryCode_ZM,
	"zimbabwe":                         constants.CountryCode_ZW,
}

// ErrUnknownSlug is returned when a FlixPatrol slug has no known ISO mapping.
type ErrUnknownSlug struct {
	Slug string
}

func (e *ErrUnknownSlug) Error() string {
	return fmt.Sprintf("unknown slug: %s", e.Slug)
}

// GetCountryCode returns the ISO 3166-1 alpha-2 code for a given FlixPatrol slug.
func GetCountryCode(slug string) (constants.CountryCode, error) {
	code, ok := slugToCode[slug]
	if !ok {
		return constants.CountryCode_XX, &ErrUnknownSlug{Slug: slug}
	}
	return code, nil
}

// GetCountryCodeFromISO validates a 2-letter ISO-3166 alpha-2 code.
func GetCountryCodeFromISO(code string) (constants.CountryCode, error) {
	upper := strings.ToUpper(strings.TrimSpace(code))
	if len(upper) != 2 {
		return constants.CountryCode_XX, &ErrUnknownSlug{Slug: code}
	}

	for _, known := range slugToCode {
		if string(known) == upper {
			return known, nil
		}
	}
	return constants.CountryCode_XX, &ErrUnknownSlug{Slug: code}
}

// GetCountrySlugFromISO returns the FlixPatrol slug for an ISO country code.
func GetCountrySlugFromISO(code string) (string, error) {
	known, err := GetCountryCodeFromISO(code)
	if err != nil {
		return "", err
	}
	for slug, mapped := range slugToCode {
		if mapped == known {
			return slug, nil
		}
	}
	return "", &ErrUnknownSlug{Slug: code}
}

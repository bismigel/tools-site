package tool

import (
	"fmt"
	"strings"
	"time"
)

// city is a resolved place: an IANA zone plus the name shown in the grid.
type city struct {
	Zone string
	Name string
}

type cityEntry struct {
	zone  string
	names []string
}

// cityList is deliberately a curated list, not a gazetteer: a recruiter needs
// their cities to resolve, not every town on earth. Any valid IANA zone is also
// accepted (see resolveCity).
var cityList = []cityEntry{
	{"Europe/London", []string{"london", "uk", "united kingdom", "england", "britain", "great britain", "manchester", "birmingham", "leeds", "bristol", "cardiff", "belfast", "edinburgh", "glasgow"}},
	{"Europe/Dublin", []string{"dublin", "ireland", "cork"}},
	{"Europe/Lisbon", []string{"lisbon", "portugal", "porto"}},
	{"Europe/Madrid", []string{"madrid", "spain", "barcelona", "valencia", "seville"}},
	{"Europe/Paris", []string{"paris", "france", "lyon", "marseille", "toulouse", "nice"}},
	{"Europe/Berlin", []string{"berlin", "germany", "munich", "munchen", "hamburg", "frankfurt", "cologne", "koln", "stuttgart", "dusseldorf"}},
	{"Europe/Amsterdam", []string{"amsterdam", "netherlands", "holland", "rotterdam", "the hague", "hague"}},
	{"Europe/Brussels", []string{"brussels", "belgium", "antwerp", "ghent"}},
	{"Europe/Zurich", []string{"zurich", "switzerland", "geneva", "basel", "bern"}},
	{"Europe/Rome", []string{"rome", "italy", "milan", "milano"}},
	{"Europe/Vienna", []string{"vienna", "austria"}},
	{"Europe/Prague", []string{"prague", "czech republic", "czechia"}},
	{"Europe/Warsaw", []string{"warsaw", "poland"}},
	{"Europe/Budapest", []string{"budapest", "hungary"}},
	{"Europe/Stockholm", []string{"stockholm", "sweden"}},
	{"Europe/Oslo", []string{"oslo", "norway"}},
	{"Europe/Copenhagen", []string{"copenhagen", "denmark"}},
	{"Europe/Helsinki", []string{"helsinki", "finland"}},
	{"Europe/Tallinn", []string{"tallinn", "estonia"}},
	{"Europe/Riga", []string{"riga", "latvia"}},
	{"Europe/Vilnius", []string{"vilnius", "lithuania"}},
	{"Europe/Athens", []string{"athens", "greece"}},
	{"Europe/Istanbul", []string{"istanbul", "turkey", "ankara"}},
	{"Europe/Moscow", []string{"moscow", "russia"}},
	{"Europe/Kyiv", []string{"kyiv", "kiev", "ukraine"}},
	{"Europe/Bucharest", []string{"bucharest", "romania"}},
	{"Europe/Sofia", []string{"sofia", "bulgaria"}},
	{"Europe/Belgrade", []string{"belgrade", "serbia"}},
	{"Europe/Zagreb", []string{"zagreb", "croatia"}},
	{"Europe/Ljubljana", []string{"ljubljana", "slovenia"}},
	{"Europe/Bratislava", []string{"bratislava", "slovakia"}},
	{"Europe/Luxembourg", []string{"luxembourg"}},
	{"Atlantic/Reykjavik", []string{"reykjavik", "iceland"}},
	{"America/New_York", []string{"new york", "nyc", "boston", "washington", "washington dc", "atlanta", "miami", "philadelphia"}},
	{"America/Toronto", []string{"toronto", "ottawa", "montreal"}},
	{"America/Chicago", []string{"chicago", "austin", "dallas", "houston"}},
	{"America/Denver", []string{"denver"}},
	{"America/Edmonton", []string{"calgary", "edmonton"}},
	{"America/Los_Angeles", []string{"los angeles", "la", "san francisco", "seattle", "san diego", "portland"}},
	{"America/Vancouver", []string{"vancouver", "victoria"}},
	{"America/Mexico_City", []string{"mexico city", "mexico"}},
	{"America/Sao_Paulo", []string{"sao paulo", "são paulo", "brazil", "rio de janeiro", "rio"}},
	{"America/Argentina/Buenos_Aires", []string{"buenos aires", "argentina"}},
	{"America/Santiago", []string{"santiago", "chile"}},
	{"America/Bogota", []string{"bogota", "colombia"}},
	{"America/Lima", []string{"lima", "peru"}},
	{"Asia/Kolkata", []string{"bangalore", "bengaluru", "mumbai", "delhi", "new delhi", "hyderabad", "pune", "chennai", "kolkata", "india"}},
	{"Asia/Karachi", []string{"karachi", "pakistan", "lahore", "islamabad"}},
	{"Asia/Dhaka", []string{"dhaka", "bangladesh"}},
	{"Asia/Colombo", []string{"colombo", "sri lanka"}},
	{"Asia/Kathmandu", []string{"kathmandu", "nepal"}},
	{"Asia/Singapore", []string{"singapore"}},
	{"Asia/Hong_Kong", []string{"hong kong"}},
	{"Asia/Shanghai", []string{"shanghai", "beijing", "shenzhen", "china"}},
	{"Asia/Taipei", []string{"taipei", "taiwan"}},
	{"Asia/Tokyo", []string{"tokyo", "japan", "osaka", "kyoto"}},
	{"Asia/Seoul", []string{"seoul", "korea", "south korea", "busan"}},
	{"Asia/Bangkok", []string{"bangkok", "thailand"}},
	{"Asia/Jakarta", []string{"jakarta", "indonesia"}},
	{"Asia/Kuala_Lumpur", []string{"kuala lumpur", "malaysia"}},
	{"Asia/Manila", []string{"manila", "philippines"}},
	{"Asia/Ho_Chi_Minh", []string{"ho chi minh city", "ho chi minh", "saigon", "vietnam", "hanoi"}},
	{"Asia/Dubai", []string{"dubai", "uae", "united arab emirates", "abu dhabi"}},
	{"Asia/Qatar", []string{"doha", "qatar"}},
	{"Asia/Riyadh", []string{"riyadh", "saudi arabia"}},
	{"Asia/Jerusalem", []string{"tel aviv", "israel", "jerusalem"}},
	{"Africa/Cairo", []string{"cairo", "egypt"}},
	{"Africa/Nairobi", []string{"nairobi", "kenya"}},
	{"Africa/Lagos", []string{"lagos", "nigeria"}},
	{"Africa/Accra", []string{"accra", "ghana"}},
	{"Africa/Johannesburg", []string{"johannesburg", "joburg", "south africa", "cape town", "pretoria"}},
	{"Africa/Casablanca", []string{"casablanca", "morocco"}},
	{"Africa/Tunis", []string{"tunis", "tunisia"}},
	{"Africa/Algiers", []string{"algiers", "algeria"}},
	{"Australia/Sydney", []string{"sydney", "canberra"}},
	{"Australia/Melbourne", []string{"melbourne"}},
	{"Australia/Brisbane", []string{"brisbane"}},
	{"Australia/Perth", []string{"perth"}},
	{"Australia/Adelaide", []string{"adelaide"}},
	{"Pacific/Auckland", []string{"auckland", "new zealand", "wellington"}},
}

var cityIndex = buildCityIndex()

func buildCityIndex() map[string]city {
	m := make(map[string]city, 256)
	for _, e := range cityList {
		name := zoneCity(e.zone)
		c := city{Zone: e.zone, Name: name}
		for _, n := range e.names {
			m[normaliseCity(n)] = c
		}
		m[normaliseCity(strings.ReplaceAll(e.zone, "_", " "))] = c
		m[normaliseCity(e.zone)] = c
	}
	return m
}

func normaliseCity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "’", "'")
	return strings.Join(strings.Fields(s), " ")
}

func zoneCity(zone string) string {
	if i := strings.LastIndex(zone, "/"); i >= 0 {
		zone = zone[i+1:]
	}
	return strings.ReplaceAll(zone, "_", " ")
}

// resolveCity maps a user string to a zone. Known aliases first, then any valid
// IANA name (e.g. "Asia/Tokyo"), so the curated list never becomes a cage.
func resolveCity(s string) (city, error) {
	if c, ok := cityIndex[normaliseCity(s)]; ok {
		return c, nil
	}
	zone := strings.ReplaceAll(strings.TrimSpace(s), " ", "_")
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "Local" || zone == "UTC" {
		return city{}, fmt.Errorf("%w: unknown city %q", ErrBadInput, strings.TrimSpace(s))
	}
	return city{Zone: loc.String(), Name: zoneCity(loc.String())}, nil
}

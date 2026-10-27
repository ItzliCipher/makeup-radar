package main

// Brand is a store whose public product catalog Makeup Radar scans.
// Kind "brand" is a single makeup brand; "shop" is a multi-brand beauty
// retailer, where each product's vendor is the real brand.
type Brand struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Domain   string   `json:"domain"`
	Kind     string   `json:"kind"`  // "brand" | "shop"
	Focus    string   `json:"focus"` // "makeup" | "mixed" (mixed = strict makeup filter)
	Currency string   `json:"currency"`
	Aliases  []string `json:"aliases,omitempty"`
	Custom   bool     `json:"custom,omitempty"`
}

// builtinBrands are makeup brands and beauty shops that run storefronts
// exposing a public product catalog. Any that stop responding are skipped
// automatically and shown as unavailable in the Sources panel.
var builtinBrands = []Brand{
	// Celebrity & viral makeup
	{ID: "rare-beauty", Name: "Rare Beauty", Domain: "www.rarebeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"rare beauty", "selena gomez"}},
	{ID: "kylie", Name: "Kylie Cosmetics", Domain: "kyliecosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"kylie cosmetics", "kylie jenner", "kylie"}},
	{ID: "fenty", Name: "Fenty Beauty", Domain: "fentybeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"fenty beauty", "fenty", "rihanna"}},
	{ID: "haus-labs", Name: "Haus Labs", Domain: "www.hauslabs.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"haus labs", "haus laboratories", "lady gaga"}},
	{ID: "rhode", Name: "Rhode", Domain: "www.rhodeskin.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"rhode", "hailey bieber"}},
	{ID: "rem-beauty", Name: "r.e.m. beauty", Domain: "rembeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"rem beauty", "r e m beauty", "ariana grande"}},
	{ID: "about-face", Name: "About-Face", Domain: "aboutface.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"about face", "halsey"}},
	{ID: "gxve", Name: "GXVE Beauty", Domain: "gxvebeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"gxve", "gwen stefani"}},
	{ID: "one-size", Name: "ONE/SIZE", Domain: "onesizebeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"one size beauty", "onesize", "one size", "patrick starrr"}},
	{ID: "pleasing", Name: "Pleasing", Domain: "pleasing.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"pleasing", "harry styles"}},

	// Artist-founded & prestige
	{ID: "makeup-by-mario", Name: "Makeup by Mario", Domain: "www.makeupbymario.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"makeup by mario", "mario"}},
	{ID: "patrick-ta", Name: "Patrick Ta", Domain: "www.patrickta.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"patrick ta"}},
	{ID: "danessa-myricks", Name: "Danessa Myricks Beauty", Domain: "danessamyricksbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"danessa myricks", "danessa"}},
	{ID: "westman", Name: "Westman Atelier", Domain: "www.westman-atelier.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"westman atelier", "westman"}},
	{ID: "vb-beauty", Name: "Victoria Beckham Beauty", Domain: "www.victoriabeckhambeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"victoria beckham beauty", "victoria beckham"}},
	{ID: "pat-mcgrath", Name: "Pat McGrath Labs", Domain: "www.patmcgrath.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"pat mcgrath labs", "pat mcgrath"}},
	{ID: "natasha-denona", Name: "Natasha Denona", Domain: "www.natashadenona.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"natasha denona"}},
	{ID: "isamaya", Name: "Isamaya Beauty", Domain: "isamayabeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"isamaya"}},
	{ID: "hourglass", Name: "Hourglass", Domain: "www.hourglasscosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"hourglass cosmetics", "hourglass"}},
	{ID: "violette-fr", Name: "Violette_FR", Domain: "violettefr.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"violette fr", "violette"}},
	{ID: "hindash", Name: "Hindash", Domain: "hindashcosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"hindash"}},
	{ID: "laura-geller", Name: "Laura Geller", Domain: "www.laurageller.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"laura geller"}},
	{ID: "thrive", Name: "Thrive Causemetics", Domain: "thrivecausemetics.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"thrive causemetics"}},

	// Skin-first & clean makeup
	{ID: "glossier", Name: "Glossier", Domain: "www.glossier.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"glossier"}},
	{ID: "kosas", Name: "Kosas", Domain: "kosas.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"kosas"}},
	{ID: "ilia", Name: "ILIA", Domain: "iliabeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"ilia beauty", "ilia"}},
	{ID: "saie", Name: "Saie", Domain: "saiehello.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"saie"}},
	{ID: "tower-28", Name: "Tower 28", Domain: "www.tower28beauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"tower 28", "tower28"}},
	{ID: "merit", Name: "Merit", Domain: "www.meritbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"merit beauty", "merit"}},
	{ID: "jones-road", Name: "Jones Road", Domain: "www.jonesroadbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"jones road"}},
	{ID: "milk", Name: "Milk Makeup", Domain: "milkmakeup.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"milk makeup"}},
	{ID: "rms", Name: "RMS Beauty", Domain: "www.rmsbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"rms beauty"}},
	{ID: "kjaer-weis", Name: "Kjaer Weis", Domain: "kjaerweis.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"kjaer weis"}},
	{ID: "exa", Name: "Exa Beauty", Domain: "exabeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"exa beauty"}},
	{ID: "vapour", Name: "Vapour Beauty", Domain: "www.vapourbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"vapour beauty"}},
	{ID: "ere-perez", Name: "Ere Perez", Domain: "ereperez.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"ere perez"}},
	{ID: "lawless", Name: "Lawless Beauty", Domain: "www.lawlessbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"lawless beauty"}},
	{ID: "supergoop", Name: "Supergoop!", Domain: "supergoop.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"supergoop"}},
	{ID: "summer-fridays", Name: "Summer Fridays", Domain: "summerfridays.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"summer fridays"}},
	{ID: "nudestix", Name: "Nudestix", Domain: "www.nudestix.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"nudestix"}},
	{ID: "florence-by-mills", Name: "Florence by Mills", Domain: "florencebymills.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"florence by mills", "millie bobby brown"}},

	// Color-first & creator brands
	{ID: "colourpop", Name: "ColourPop", Domain: "colourpop.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"colourpop", "colour pop", "colorpop"}},
	{ID: "jeffree-star", Name: "Jeffree Star Cosmetics", Domain: "jeffreestarcosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"jeffree star cosmetics", "jeffree star"}},
	{ID: "lime-crime", Name: "Lime Crime", Domain: "www.limecrime.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"lime crime"}},
	{ID: "melt", Name: "Melt Cosmetics", Domain: "meltcosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"melt cosmetics"}},
	{ID: "morphe", Name: "Morphe", Domain: "www.morphe.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"morphe"}},
	{ID: "half-magic", Name: "Half Magic", Domain: "halfmagicbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"half magic"}},
	{ID: "glamnetic", Name: "Glamnetic", Domain: "glamnetic.com", Kind: "brand", Focus: "mixed", Currency: "USD", Aliases: []string{"glamnetic"}},
	{ID: "kaja", Name: "Kaja", Domain: "kajabeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"kaja beauty", "kaja"}},
	{ID: "dibs", Name: "Dibs Beauty", Domain: "dibsbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"dibs beauty"}},
	{ID: "freck", Name: "Freck Beauty", Domain: "freckbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"freck beauty", "freck"}},

	// Inclusive-shade pioneers
	{ID: "juvias-place", Name: "Juvia's Place", Domain: "www.juviasplace.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"juvias place"}},
	{ID: "ami-cole", Name: "Ami Colé", Domain: "amicole.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"ami cole"}},
	{ID: "kulfi", Name: "Kulfi", Domain: "kulfibeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"kulfi beauty", "kulfi"}},
	{ID: "lys", Name: "LYS Beauty", Domain: "lysbeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"lys beauty"}},
	{ID: "uoma", Name: "UOMA Beauty", Domain: "uomabeauty.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"uoma beauty", "uoma"}},
	{ID: "mented", Name: "Mented Cosmetics", Domain: "mentedcosmetics.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"mented cosmetics", "mented"}},
	{ID: "beauty-bakerie", Name: "Beauty Bakerie", Domain: "beautybakerie.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"beauty bakerie"}},

	// UK makeup
	{ID: "refy", Name: "Refy", Domain: "refybeauty.com", Kind: "brand", Focus: "makeup", Currency: "GBP", Aliases: []string{"refy"}},
	{ID: "made-by-mitchell", Name: "Made by Mitchell", Domain: "madebymitchell.co.uk", Kind: "brand", Focus: "makeup", Currency: "GBP", Aliases: []string{"made by mitchell"}},
	{ID: "iconic-london", Name: "Iconic London", Domain: "iconiclondoninc.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"iconic london"}},
	{ID: "ciate", Name: "Ciaté London", Domain: "ciatelondon.com", Kind: "brand", Focus: "makeup", Currency: "USD", Aliases: []string{"ciate london", "ciate"}},

	// Multi-brand beauty shops (makeup filtered from their full catalogs)
	{ID: "credo", Name: "Credo Beauty", Domain: "credobeauty.com", Kind: "shop", Focus: "mixed", Currency: "USD"},
	{ID: "violet-grey", Name: "Violet Grey", Domain: "www.violetgrey.com", Kind: "shop", Focus: "mixed", Currency: "USD"},
	{ID: "thirteen-lune", Name: "Thirteen Lune", Domain: "www.thirteenlune.com", Kind: "shop", Focus: "mixed", Currency: "USD"},
	{ID: "soko-glam", Name: "Soko Glam", Domain: "sokoglam.com", Kind: "shop", Focus: "mixed", Currency: "USD"},
	{ID: "detox-market", Name: "The Detox Market", Domain: "www.thedetoxmarket.com", Kind: "shop", Focus: "mixed", Currency: "USD"},
}

// otherBrands are major makeup brands that don't publish a scannable
// catalog. They're still recognised in searches and in launch news.
var otherBrands = map[string][]string{
	"Dior":                    {"dior", "dior beauty"},
	"Chanel":                  {"chanel"},
	"YSL Beauty":              {"ysl", "ysl beauty", "yves saint laurent"},
	"Armani Beauty":           {"armani beauty", "giorgio armani beauty", "armani"},
	"Charlotte Tilbury":       {"charlotte tilbury"},
	"NARS":                    {"nars"},
	"MAC Cosmetics":           {"mac cosmetics", "m a c cosmetics"},
	"Benefit":                 {"benefit cosmetics"},
	"Huda Beauty":             {"huda beauty", "huda"},
	"e.l.f.":                  {"elf cosmetics", "e l f", "elf beauty", "e l f cosmetics", "e l f beauty"},
	"Maybelline":              {"maybelline"},
	"L'Oréal Paris":           {"loreal", "loreal paris", "l oreal"},
	"NYX":                     {"nyx", "nyx professional makeup"},
	"Too Faced":               {"too faced"},
	"Tarte":                   {"tarte", "tarte cosmetics"},
	"Urban Decay":             {"urban decay"},
	"Laneige":                 {"laneige"},
	"Clinique":                {"clinique"},
	"Estée Lauder":            {"estee lauder"},
	"Lancôme":                 {"lancome"},
	"Givenchy Beauty":         {"givenchy"},
	"Guerlain":                {"guerlain"},
	"Hermès Beauty":           {"hermes beauty", "hermes"},
	"Gucci Beauty":            {"gucci beauty"},
	"Prada Beauty":            {"prada beauty"},
	"Valentino Beauty":        {"valentino beauty"},
	"Tom Ford Beauty":         {"tom ford beauty", "tom ford"},
	"Byredo":                  {"byredo"},
	"Rabanne":                 {"rabanne"},
	"Sisley":                  {"sisley"},
	"Shiseido":                {"shiseido"},
	"Clé de Peau":             {"cle de peau"},
	"Make Up For Ever":        {"make up for ever", "makeup forever", "make up forever"},
	"Smashbox":                {"smashbox"},
	"Bobbi Brown":             {"bobbi brown"},
	"Laura Mercier":           {"laura mercier"},
	"IT Cosmetics":            {"it cosmetics"},
	"Anastasia Beverly Hills": {"anastasia beverly hills", "abh"},
	"Milani":                  {"milani"},
	"wet n wild":              {"wet n wild"},
	"Essence":                 {"essence cosmetics"},
	"Catrice":                 {"catrice"},
	"CoverGirl":               {"covergirl"},
	"Revlon":                  {"revlon"},
	"Rimmel":                  {"rimmel"},
	"KIKO Milano":             {"kiko milano", "kiko"},
	"Makeup Revolution":       {"makeup revolution", "revolution beauty"},
	"SHEGLAM":                 {"sheglam"},
	"rom&nd":                  {"romand", "rom nd", "rom and nd"},
	"Peripera":                {"peripera"},
	"CLIO":                    {"clio cosmetics"},
	"hince":                   {"hince"},
	"TIRTIR":                  {"tirtir"},
	"fwee":                    {"fwee"},
	"3CE":                     {"3ce"},
	"Innisfree":               {"innisfree"},
	"Sulwhasoo":               {"sulwhasoo"},
	"Dolce & Gabbana Beauty":  {"dolce and gabbana beauty", "dolce gabbana beauty"},
	"Sephora Collection":      {"sephora collection"},
	"Physicians Formula":      {"physicians formula"},
	"Tatcha":                  {"tatcha"},
	"Glow Recipe":             {"glow recipe"},
}

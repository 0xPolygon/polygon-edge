package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

func OsFilePermissionRule(m dsl.Matcher) {
	m.Match(`os.$name($file, $number, 0777)`,
		`os.$name($file, 0777)`,
		`os.$name(0777)`,
		`os.$name($file, $number, os.ModePerm)`,
		`os.$name($file, os.ModePerm)`,
		`os.$name(os.ModePerm)`,
	).Report("os.$name called with file mode 0777 (ModePerm)")
}

func ForbidPanicsRule(m dsl.Matcher) {
	m.Match(`panic($_)`).Report("panics should not be manually used")
}

func NoUseOfTimeNowWithoutUTCRule(m dsl.Matcher) {
	m.Match(`$_ = time.Now()`,
		`$_ := time.Now()`,
		`$_.$_ = time.Now()`).
		Suggest("time.Now().UTC()").
		Report(`use UTC time when calling time.Now`)

	m.Match(`time.Now().$f()`,
		`time.Now().$f($_)`,
		`time.Now().$f($_, $_)`,
		`time.Now().$f($_, $_, $_)`).
		Where(!m["f"].Text.Matches("UTC")).
		Suggest("time.Now().UTC()").
		Report(`use UTC time when calling time.Now`)
}

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

func OsFilePermissionRule(m dsl.Matcher) {
	m.Match(`os.$name($file, $number, 0777)`).Report("os.$name called with file mode 0777")
	m.Match(`os.$name($file, 0777)`).Report("os.$name called with file mode 0777")
	m.Match(`os.$name(0777)`).Report("os.$name called with file mode 0777")

	m.Match(`os.$name($file, $number, os.ModePerm)`).Report("os.$name called with file mode os.ModePerm (0777)")
	m.Match(`os.$name($file, os.ModePerm)`).Report("os.$name called with file mode os.ModePerm (0777)")
	m.Match(`os.$name(os.ModePerm)`).Report("os.$name called with file mode os.ModePerm (0777)")
}

func ForbidPanicsRule(m dsl.Matcher) {
	m.Match(`panic($_)`).Report("panics should not be manually used")
}

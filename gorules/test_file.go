package gorules

import "os"

func testBre() error {
	return os.Mkdir("aaa", 0777)
}

func testBre2() {
	os.Mkdir("aaa", os.ModePerm)
}

func testBre3() {
	os.OpenFile("aaa", 1, 0777)
}

func testBre4() {
	os.OpenFile("aaa", 1, os.ModePerm)
}

func testBre5() {
	os.OpenFile("aaa", 1, 67108864)
}

func testBre6() {
	os.OpenFile("aaa", 1, os.ModeDevice)
}

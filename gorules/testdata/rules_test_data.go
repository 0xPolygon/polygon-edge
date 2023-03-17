package target

import (
	"errors"
	"fmt"
	"os"
)

func testLintError() {
	err := errors.New("test")

	os.Mkdir("test", 0777)              // want `OsFilePermissionRule: os.Mkdir called with file mode`
	os.Mkdir("test", os.ModePerm)       // want `OsFilePermissionRule: os.Mkdir called with file mode`
	os.MkdirAll("test", 0777)           // want `OsFilePermissionRule: os.MkdirAll called with file mode`
	os.MkdirAll("test", os.ModePerm)    // want `OsFilePermissionRule: os.MkdirAll called with file mode`
	os.Chmod("test", 0777)              // want `OsFilePermissionRule: os.Chmod called with file mode`
	os.Chmod("test", os.ModePerm)       // want `OsFilePermissionRule: os.Chmod called with file mode`
	os.OpenFile("test", 0, 0777)        // want `OsFilePermissionRule: os.OpenFile called with file mode`
	os.OpenFile("test", 0, os.ModePerm) // want `OsFilePermissionRule: os.OpenFile called with file mode`
	panic("test")                       // want `ForbidPanicsRule: panics should not be manually used`
	panic(fmt.Sprintf("test error"))    // want `ForbidPanicsRule: panics should not be manually used`
	panic(err)                          // want `ForbidPanicsRule: panics should not be manually used`
}

func testNoLintError() {
	os.Mkdir("test", 67108864)
	os.Mkdir("test", os.ModeDevice)
	os.MkdirAll("test", 67108864)
	os.MkdirAll("test", os.ModeDevice)
	os.Chmod("test", 67108864)
	os.Chmod("test", os.ModeDevice)
	os.OpenFile("test", 0, 67108864)
	os.OpenFile("test", 0, os.ModeDevice)
}

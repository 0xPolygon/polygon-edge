package target

import (
	"errors"
	"fmt"
	"os"
	"time"
)

type testLintStruct struct {
	testTime time.Time
}

func testLintError() {
	err := errors.New("test")
	testStruct := &testLintStruct{}
	var timeNow time.Time

	os.Mkdir("test", 0777)                                        // want `OsFilePermissionRule: os.Mkdir called with file mode 0777`
	os.Mkdir("test", os.ModePerm)                                 // want `OsFilePermissionRule: os.Mkdir called with file mode 0777`
	os.MkdirAll("test", 0777)                                     // want `OsFilePermissionRule: os.MkdirAll called with file mode 0777`
	os.MkdirAll("test", os.ModePerm)                              // want `OsFilePermissionRule: os.MkdirAll called with file mode 0777`
	os.Chmod("test", 0777)                                        // want `OsFilePermissionRule: os.Chmod called with file mode 0777`
	os.Chmod("test", os.ModePerm)                                 // want `OsFilePermissionRule: os.Chmod called with file mode 0777`
	os.OpenFile("test", 0, 0777)                                  // want `OsFilePermissionRule: os.OpenFile called with file mode 0777`
	os.OpenFile("test", 0, os.ModePerm)                           // want `OsFilePermissionRule: os.OpenFile called with file mode 0777`
	panic("test")                                                 // want `ForbidPanicsRule: panics should not be manually used`
	panic(fmt.Sprintf("test error"))                              // want `ForbidPanicsRule: panics should not be manually used`
	panic(err)                                                    // want `ForbidPanicsRule: panics should not be manually used`
	testStruct.testTime = time.Now()                              // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	timeNow = time.Now()                                          // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	v := time.Now()                                               // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Unix()                                             // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().UnixMicro()                                        // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().UnixMilli()                                        // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().UnixNano()                                         // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Add(time.Hour)                                     // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().AddDate(1, 1, 1)                                   // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().After(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local))  // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().AppendFormat([]byte{0, 1}, "dd.mm.yyyy")           // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Before(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local)) // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Clock()                                            // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Date()                                             // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Day()                                              // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Equal(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local))  // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Hour()                                             // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Minute()                                           // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Hour()                                             // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Year()                                             // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`
	time.Now().Add(time.Hour).Unix()                              // want `NoUseOfTimeNowWithoutUTCRule: use UTC time when calling time.Now`

	v.Day()
	timeNow.Day()
}

func testNoLintError() {
	testStruct := &testLintStruct{}
	var timeNow time.Time

	os.Mkdir("test", 67108864)
	os.Mkdir("test", os.ModeDevice)
	os.MkdirAll("test", 67108864)
	os.MkdirAll("test", os.ModeDevice)
	os.Chmod("test", 67108864)
	os.Chmod("test", os.ModeDevice)
	os.OpenFile("test", 0, 67108864)
	os.OpenFile("test", 0, os.ModeDevice)
	testStruct.testTime = time.Now().UTC()
	timeNow = time.Now().UTC()
	v := time.Now().UTC()
	time.Now().UTC().Unix()
	time.Now().UTC().UnixMicro()
	time.Now().UTC().UnixMilli()
	time.Now().UTC().UnixNano()
	time.Now().UTC().Add(time.Hour)
	time.Now().UTC().AddDate(1, 1, 1)
	time.Now().UTC().After(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local))
	time.Now().UTC().AppendFormat([]byte{0, 1}, "dd.mm.yyyy")
	time.Now().UTC().Before(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local))
	time.Now().UTC().Clock()
	time.Now().UTC().Date()
	time.Now().UTC().Day()
	time.Now().UTC().Equal(time.Date(1, 1, 1, 1, 1, 1, 1, time.Local))
	time.Now().UTC().Hour()
	time.Now().UTC().Minute()
	time.Now().UTC().Hour()
	time.Now().UTC().Year()
	time.Now().UTC().Add(time.Hour).Unix()

	v.Day()
	timeNow.Day()
}

package export

const (
	fileTypeFlag = "type"
)

type exportParams struct {
	FileType string
}

var (
	paramFlagValues = &exportParams{}
)

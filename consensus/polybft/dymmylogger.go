package polybft

type DummyLogger struct {
}

func (d *DummyLogger) Printf(format string, args ...interface{}) {
}

func (d *DummyLogger) Print(args ...interface{}) {
}

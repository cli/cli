package test

// OutputStub implements a simple utils.Runnable
type OutputStub struct {
	Out []byte
}

func (s OutputStub) Output() ([]byte, error) {
	return s.Out, nil
}

func (s OutputStub) Run() error {
	return nil
}

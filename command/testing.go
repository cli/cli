package command

// outputStub implements a simple utils.Runnable
type outputStub struct {
	output []byte
}

func (s outputStub) Output() ([]byte, error) {
	return s.output, nil
}

func (s outputStub) Run() error {
	return nil
}

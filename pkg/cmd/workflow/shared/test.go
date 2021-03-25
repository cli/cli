package shared

var AWorkflow = Workflow{
	Name:  "a workflow",
	ID:    123,
	Path:  ".github/workflows/flow.yml",
	State: Active,
}
var AWorkflowContent = `{"content":"bmFtZTogYSB3b3JrZmxvdwo="}`

var DisabledWorkflow = Workflow{
	Name:  "a disabled workflow",
	ID:    456,
	Path:  ".github/workflows/disabled.yml",
	State: DisabledManually,
}

var AnotherDisabledWorkflow = Workflow{
	Name:  "a disabled workflow",
	ID:    1213,
	Path:  ".github/workflows/anotherDisabled.yml",
	State: DisabledManually,
}

var UniqueDisabledWorkflow = Workflow{
	Name:  "terrible workflow",
	ID:    1314,
	Path:  ".github/workflows/terrible.yml",
	State: DisabledManually,
}

var AnotherWorkflow = Workflow{
	Name:  "another workflow",
	ID:    789,
	Path:  ".github/workflows/another.yml",
	State: Active,
}
var AnotherWorkflowContent = `{"content":"bmFtZTogYW5vdGhlciB3b3JrZmxvdwo="}`

var YetAnotherWorkflow = Workflow{
	Name:  "another workflow",
	ID:    1011,
	Path:  ".github/workflows/yetanother.yml",
	State: Active,
}

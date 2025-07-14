package main

// TestInterface is a simple interface for testing
type TestInterface interface {
	TestMethod() string
}

// TestStruct implements TestInterface
type TestStruct struct {
	Name string
}

// TestMethod implements TestInterface
func (t *TestStruct) TestMethod() string {
	return t.Name
}

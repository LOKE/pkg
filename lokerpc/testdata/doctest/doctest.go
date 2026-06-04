// Package doctest provides test fixtures for documentation extraction.
package doctest

// Person represents a person in the system.
type Person struct {
	// Name is the full name of the person.
	Name string `json:"name"`
	// Age is the person's age in years.
	Age int `json:"age"`
	// Email is an optional contact email.
	Email string `json:"email,omitempty"`
}

// Request is a test request type.
type Request struct {
	// ID is the unique identifier.
	ID string `json:"id"`
	// Data contains the request payload.
	Data string `json:"data"`
}

// UndocumentedType has no documentation.
type UndocumentedType struct {
	Field1 string `json:"field1"`
	Field2 string `json:"field2"`
}

package p

// Currency is an enum type: a named string with an EnumValues method
// (i.e. it implements lokerpc.EnumProvider).
type Currency string

func (Currency) EnumValues() []string { return []string{"AUD", "NZD", "USD"} }

// plainString is a named string with no EnumValues method, so it is not an enum.
type plainString string

type Good struct {
	Currency Currency `json:"currency" validate:"enum"`
}

type GoodOptional struct {
	Currency *Currency `json:"currency" validate:"omitempty,enum"`
}

type Untagged struct {
	Currency Currency `json:"currency"` // want `field Currency is an enum type \(Currency\) but its validate tag is missing the "enum" rule`
}

type MissingEnumRule struct {
	Currency *Currency `validate:"required"` // want `field Currency is an enum type \(Currency\) but its validate tag is missing the "enum" rule`
}

type NoTag struct {
	Currency Currency // want `field Currency is an enum type \(Currency\) but its validate tag is missing the "enum" rule`
}

// NotEnumFields must not be flagged: neither field is an enum type.
type NotEnumFields struct {
	Name  string      `json:"name"`
	Plain plainString `json:"plain"`
}

// unexportedEnum fields are skipped: they can't be validated by tag anyway.
type unexportedEnum struct {
	currency Currency `json:"currency"`
}

// CurrencyAlias is a type alias; enum detection must unwrap it (Go 1.23+).
type CurrencyAlias = Currency

type AliasField struct {
	Currency CurrencyAlias `json:"currency"` // want `field Currency is an enum type \(Currency\) but its validate tag is missing the "enum" rule`
}

// enumLike is a named interface that declares EnumValues. It must NOT be treated
// as an enum: only named string types are. A field of this type is not flagged.
type enumLike interface {
	EnumValues() []string
}

type InterfaceField struct {
	Provider enumLike `json:"provider"`
}

// Embedded enum fields carry no field name, so the analyzer does not flag them
// at the embedding site. This fixture documents that known limitation.
type Embedded struct {
	Currency
}

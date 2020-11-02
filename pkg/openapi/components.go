package openapi

func NewComponents() Components {
	return Components{
		Schemas:    Schemas{},
		Parameters: Parameters{},
	}
}

// Components is used to store shared data between
// various parts of the openapi doc
type Components struct {
	Parameters Parameters
	Schemas    Schemas
}

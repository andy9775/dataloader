package dataloader

// Result is an alias for the resolved data by the batch loader
type Result interface{}

// ResultMap maps each loaded elements Result against the elements unique identifier (Key)
type ResultMap map[Identifier]Result

func (r *ResultMap) isEmpty() bool {
	return len(*r) == 0
}

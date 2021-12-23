package dscope

type Defs []any

func (d *Defs) Add(defs ...any) {
	*d = append(*d, defs...)
}

func (d Defs) NewScope() Scope {
	return New(d...)
}

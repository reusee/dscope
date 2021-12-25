package dscope

type Defs []any

func (d *Defs) Add(defs ...any) *Defs {
	*d = append(*d, defs...)
	return d
}

func (d Defs) Copy() []any {
	ret := make([]any, len(d))
	copy(ret, d)
	return ret
}

func (d Defs) NewScope(defs ...any) Scope {
	defs = append(d.Copy(), defs...)
	return New(defs...)
}

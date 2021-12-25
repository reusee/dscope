package dscope

type Defs []any

func (d *Defs) Add(defs ...any) {
	*d = append(*d, defs...)
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

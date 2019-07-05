package dscope

import "bytes"

type _SubFuncsSpec struct {
	Sig1 []byte
	Sig2 []byte
	Func _SubFunc
}

type _SubFuncsMap []_SubFuncsSpec

func (m _SubFuncsMap) Insert(spec _SubFuncsSpec) _SubFuncsMap {
	left := uint(0)
	right := uint(len(m))
	var i uint
	var s2 _SubFuncsSpec
	for left < right {
		i = (left + right) >> 1
		s2 = m[i]
		if c1 := bytes.Compare(s2.Sig1, spec.Sig1); c1 == 0 {
			if c2 := bytes.Compare(s2.Sig2, spec.Sig2); c2 == -1 {
				// s2.sig2 < spec.sig2
				left = i + 1
			} else if c2 == 1 {
				// s2.sig2 > spec.sig2
				right = i
			} else {
				// already existed
				return m
			}
		} else if c1 == -1 {
			// s2.sig1 < spec.sig1
			left = i + 1
		} else {
			// s2.sig1 > spec.sig1
			right = i
		}
	}
	return append(
		m[:left],
		append(
			_SubFuncsMap{spec},
			m[left:]...,
		)...,
	)
}

func (m _SubFuncsMap) Find(sig1 []byte, sig2 []byte) (spec _SubFuncsSpec, ok bool) {
	left := uint(0)
	right := uint(len(m))
	var i uint
	var s2 _SubFuncsSpec
	for left < right {
		i = (left + right) >> 1
		s2 = m[i]
		if c1 := bytes.Compare(s2.Sig1, sig1); c1 == 0 {
			if c2 := bytes.Compare(s2.Sig2, sig2); c2 == -1 {
				// s2.sig2 < sig2
				left = i + 1
			} else if c2 == 1 {
				// s2.sig2 > sig2
				right = i
			} else {
				// found
				spec = s2
				ok = true
				return
			}
		} else if c1 == -1 {
			// s2.sig1 < sig1
			left = i + 1
		} else {
			// s2.sig1 > sig1
			right = i
		}
	}
	return
}

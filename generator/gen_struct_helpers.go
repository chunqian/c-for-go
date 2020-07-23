package generator

import (
	"bytes"
	"fmt"
	"hash/crc32"

	tl "github.com/xlab/c-for-go/translator"
)

func (gen *Generator) getStructHelpers(goStructName []byte, cStructName string, spec tl.CType) (helpers []*Helper) {
	crc := getRefCRC(spec)
	cgoSpec := gen.tr.CGoSpec(spec, true)

	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "func (x *%s) Ref() *%s", goStructName, cgoSpec)
	fmt.Fprintf(buf, `{
		if x == nil {
			return nil
		}
		return x.ref%2x
	}`, crc)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Ref", goStructName),
		Description: "Ref returns the underlying reference to C object or nil if struct is nil.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) ResetRef()", goStructName)
	fmt.Fprintf(buf, `{
		if x == nil {
			return
		}
		x.ref%2x = nil
	}`, crc)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.ResetRef", goStructName),
		Description: "ResetRef set ref nil if memory freed by CGo call C function.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) FreeRef()", goStructName)
	fmt.Fprintf(buf, `{
		if x != nil && x.allocs%2x != nil {
			x.allocs%2x.(*cgoAllocMap).Free()
			x.ref%2x = nil
			return
		}
		if x != nil && x.ref%2x != nil && x.allocs%2x == nil {
			C.free(unsafe.Pointer(x.ref%2x))
			x.ref%2x = nil
			return
		}
	}`, crc, crc, crc, crc, crc, crc, crc)
	helpers = append(helpers, &Helper{
		Name: fmt.Sprintf("%s.FreeRef", goStructName),
		Description: "FreeRef invokes alloc map's free mechanism that cleanups any allocated memory using C free.\n" +
			"Does nothing if struct is nil or has no allocation map.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func free%s(x *%s)", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		if x != nil && x.allocs%2x != nil {
			x.allocs%2x.(*cgoAllocMap).Free()
			x.ref%2x = nil
			// fmt.Printf("%s memory: %%p free\n", x)
			// return
		}
		gc.mux.Lock() // gc lock
		a := x.allocs%2x.(*cgoAllocMap)
		if gc.references == nil {
			return
		}
		for ptr := range a.m {
			// C.free(ptr)
			// delete(a.m, ptr)
			if _, ok := gc.references[ptr]; ok {
				gc.references[ptr].count -= 1
				if gc.references[ptr].count == 0 {
					fmt.Printf("%s memory: %%p free\n", ptr)
					C.free(ptr)
					delete(gc.references, ptr)
					fmt.Printf("del reference, still exist: %%d\n", len(gc.references))
				}
			}
		}
		gc.mux.Unlock() // gc unlock
	}`, crc, crc, crc, goStructName, crc, cgoSpec.Base)
	helpers = append(helpers, &Helper{
		Name: fmt.Sprintf("free%s", goStructName),
		Description: "Auto free invokes alloc map's free mechanism that cleanups any allocated memory using C free.\n" +
			"Does nothing if struct is nil or has no allocation map.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func New%sRef(ref unsafe.Pointer) *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		if ref == nil {
			return nil
		}
		obj := new(%s)
		obj.ref%2x = (*%s)(unsafe.Pointer(ref))
		return obj
	}`, goStructName, crc, cgoSpec)

	name := fmt.Sprintf("New%sRef", goStructName)
	helpers = append(helpers, &Helper{
		Name: name,
		Description: name + " creates a new wrapper struct with underlying reference set to the original C object.\n" +
			"Returns nil if the provided pointer to C object is nil too.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) PassRef() (*%s, *cgoAllocMap) {\n", goStructName, cgoSpec)
	buf.Write(gen.getPassRefSource(goStructName, cStructName, spec))
	buf.WriteRune('}')
	helpers = append(helpers, &Helper{
		Name: fmt.Sprintf("%s.PassRef", goStructName),
		Description: "PassRef returns the underlying C object, otherwise it will allocate one and set its values\n" +
			"from this wrapping struct, counting allocations into an allocation map.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x %s) Pass() *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		if x.ref%2x != nil {
			return &x
		}
		x.PassRef()
		return &x
	}`, crc)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Pass", goStructName),
		Description: "Pass does the same as PassRef except that it will try to dereference the returned pointer.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x %s) PassValue() (%s, *cgoAllocMap) {\n", goStructName, cgoSpec)
	buf.Write(gen.getPassValueSource(goStructName, spec))
	buf.WriteRune('}')
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.PassValue", goStructName),
		Description: "PassValue does the same as PassRef except that it will try to dereference the returned pointer.",
		Source:      buf.String(),
	})

	// buf.Reset()
	// fmt.Fprintf(buf, "func (x *%s) Deref() {\n", goStructName)
	// buf.Write(gen.getDerefSource(goStructName, cStructName, spec))
	// buf.WriteRune('}')
	// helpers = append(helpers, &Helper{
	// 	Name: fmt.Sprintf("%s.Deref", goStructName),
	// 	Description: "Deref uses the underlying reference to C object and fills the wrapping struct with values.\n" +
	// 		"Do not forget to call this method whether you get a struct for C object and want to read its values.",
	// 	Source: buf.String(),
	// })

	// More
	if spec.GetPointers() > 0 {
		return nil // can't addess a pointer receiver
	}
	// cgoSpec := gen.tr.CGoSpec(spec, true)
	structSpec := spec.(*tl.CStructSpec)

	if !gen.cfg.Options.StructAccessors {
		return
	}
	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
		}
		buf.Reset()
		typeName := m.Spec.GetBase()
		switch m.Spec.Kind() {
		case tl.StructKind, tl.OpaqueStructKind, tl.UnionKind, tl.EnumKind:
			if !gen.tr.IsAcceptableName(tl.TargetType, typeName) {
				continue
			}
		}
		memTip := memTipRx.TipAt(i)
		if !memTip.IsValid() {
			memTip = gen.MemTipOf(m)
		}
		ptrTip := ptrTipRx.TipAt(i)
		if memTip == tl.TipMemRaw {
			ptrTip = tl.TipPtrSRef
		}
		typeTip := typeTipRx.TipAt(i)
		goSpec := gen.tr.TranslateSpec(m.Spec, ptrTip, typeTip)
		cgoSpec := gen.tr.CGoSpec(m.Spec, false)
		// does not work for function pointers
		if goSpec.Pointers > 0 && goSpec.Base == "func" {
			continue
		}
		const public = true
		goName := string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		arr := len(goSpec.OuterArr.Sizes()) > 0 || len(goSpec.InnerArr.Sizes()) > 0
		if !arr {
			goSpec.Pointers += 1
			cgoSpec.Pointers += 1
		}

		switch {
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 0:
			fmt.Fprintf(buf, "func (s *%s) Ref%s() %s {\n", goStructName, goName, goSpec)
			toProxy, _ := gen.proxyValueToGo(memTip, "ret", "&s.Ref()."+m.Name, goSpec, cgoSpec)
			fmt.Fprintf(buf, "\tvar ret %s\n", goSpec)
			fmt.Fprintf(buf, "\t%s\n", toProxy)
			fmt.Fprintf(buf, "\treturn ret\n")
			fmt.Fprintf(buf, "}\n")
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 1:
			goSpec.Pointers -= 1
			cgoSpec.Pointers -= 1
			fmt.Fprintf(buf, "func (s *%s) Ref%s(%sCount int32) %s {\n", goStructName, goName, m.Name, goSpec)
			toProxy, _ := gen.proxyValueToGo(memTip, "ret", "s.Ref()."+m.Name, goSpec, cgoSpec)
			fmt.Fprintf(buf, "\tvar ret %s\n", goSpec)
			fmt.Fprintf(buf, "\tret = make(%s, %sCount)\n", goSpec, m.Name)
			fmt.Fprintf(buf, "\t%s\n", toProxy)
			fmt.Fprintf(buf, "\treturn ret\n")
			fmt.Fprintf(buf, "}\n")
			goSpec.Pointers += 1
			cgoSpec.Pointers += 1
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 2:
			goSpecS2P0 := goSpec
			goSpecS2P0.Pointers -= 1

			goSpecS1P0 := goSpec
			goSpecS1P0.Pointers -= 1
			goSpecS1P0.Slices -= 1

			cgoSpecP2 := cgoSpec
			cgoSpecP2.Pointers -= 1

			cgoSpecP1 := cgoSpec
			cgoSpecP1.Pointers -= 2

			cgoSpecP0 := cgoSpec
			cgoSpecP0.Pointers -= 3
			fmt.Fprintf(buf, "func (s *%s) Ref%s(%sRow int32, %sColumn int32) %s", goStructName, goName, m.Name, m.Name, goSpecS2P0)
			fmt.Fprintf(buf, `{
					row, column := %sRow, %sColumn
					ret := make(%s, row)
					for i := range ret {
						ret[i] = make(%s, column)
					}
					ptr0 := s.Ref().%s
					for i0 := range ret {
						ptr1 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(i0)*uintptr(sizeOfPtr)))
						for i1 := range ret[i0] {
							ptr2 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(*ptr1)) + uintptr(i1)*uintptr(sizeOf%sValue)))
							ret[i0][i1] = *New%sRef(unsafe.Pointer(ptr2))
						}
					}
					return ret
				}`, m.Name, m.Name, goSpecS2P0, goSpecS1P0, m.Name, cgoSpecP2, cgoSpecP1, m.Spec.GetBase(), m.Spec.GetBase())
		case goSpec.Kind == tl.PlainTypeKind && goSpec.Slices > 0:
			fmt.Fprintf(buf, "func (s *%s) Ref%s(%sCount int32) %s {\n", goStructName, goName, m.Name, goSpec)
			toProxy, _ := gen.proxyValueToGo(memTip, "ret", m.Name, goSpec, cgoSpec)
			fmt.Fprintf(buf, "\tvar ret %s\n", goSpec)
			fmt.Fprintf(buf, "\t%s\n", toProxy)
			fmt.Fprintf(buf, "\treturn ret\n")
			fmt.Fprintf(buf, "}\n")
		case goSpec.Kind == tl.PlainTypeKind && goSpec.Slices == 0:
			fmt.Fprintf(buf, "func (s *%s) Ref%s() %s {\n", goStructName, goName, goSpec)
			toProxy, _ := gen.proxyValueToGo(memTip, "ret", "&s.Ref()."+m.Name, goSpec, cgoSpec)
			fmt.Fprintf(buf, "\tvar ret %s\n", goSpec)
			fmt.Fprintf(buf, "\t%s\n", toProxy)
			fmt.Fprintf(buf, "\treturn ret\n")
			fmt.Fprintf(buf, "}\n")
		}
		
		helpers = append(helpers, &Helper{
			Name:        fmt.Sprintf("%s.Ref%s", goStructName, goName),
			Description: fmt.Sprintf("Ref%s returns a reference to C object within a struct", goName),
			Source:      buf.String(),
		})
		buf.Reset()

		goSpecName := fmt.Sprintf("%s", goSpec)
		arr = len(goSpec.OuterArr.Sizes()) > 0 || len(goSpec.InnerArr.Sizes()) > 0
		if !arr {
			goSpec.Pointers -= 1
			cgoSpec.Pointers -= 1
		}

		switch {
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 0:
			fmt.Fprintf(buf, "func (s *%s) SetRef%s(%s %s) (*%s)", goStructName, goName, m.Name, goSpecName, goStructName)
			fmt.Fprintf(buf, `{
					if s.Ref() == nil { return nil }
						if %s.Ref() == nil {
							__ret, _ := %s.PassRef()
							s.Ref().%s = *__ret
						} else {
							s.Ref().%s = *%s.Ref()
						}
						return s
				}`, m.Name, m.Name, m.Name, m.Name, m.Name)
		case goSpec.Kind == tl.StructKind && goSpec.Slices > 0:
			goSpec.Slices = 0
			goSpecName = fmt.Sprintf("%s", goSpec)
			sizeConst := "sizeOfPtr"
			fmt.Fprintf(buf, "func (s *%s) SetRef%s(%sIndex int32, %s %s) (*%s) {\n", goStructName, goName, m.Name, unexportName(goSpecName), goSpec, goStructName)
			fmt.Fprintf(buf, "const m = %s\n", gen.maxMem)
			fmt.Fprintf(buf, "if s.Ref() == nil { return nil }\n")
			fmt.Fprintf(buf, "(*(*[m/%s]*%s)(unsafe.Pointer(&s.Ref().%s)))[%sIndex] = %s.Ref()\n", sizeConst, cgoSpec.Base, m.Name, m.Name, unexportName(goSpecName))
			fmt.Fprintf(buf, "return s\n")
			fmt.Fprintf(buf, "}\n")

		case goSpec.Kind == tl.PlainTypeKind && goSpec.Slices == 0:
			fmt.Fprintf(buf, "func (s *%s) SetRef%s(%s %s) (*%s) {\n", goStructName, goName, m.Name, goSpec, goStructName)
			fromProxy, _ := gen.proxyValueFromGoEx(memTip, m.Name, goSpec, cgoSpec)
			fmt.Fprintf(buf, "if s.Ref() == nil { return nil }\n")
			fmt.Fprintf(buf, "\ts.Ref().%s = %s\n", m.Name, fromProxy)
			fmt.Fprintf(buf, "return s\n")
			fmt.Fprintf(buf, "}\n")
		}

		helpers = append(helpers, &Helper{
			Name:        fmt.Sprintf("%s.SetRef%s", goStructName, goName),
			Description: fmt.Sprintf("SetRef%s update C object and binding struct", goName),
			Source:      buf.String(),
		})
	}
	return
}

func (gen *Generator) getRawStructHelpers(goStructName []byte, cStructName string, spec tl.CType) (helpers []*Helper) {
	if spec.GetPointers() > 0 {
		return nil // can't addess a pointer receiver
	}
	cgoSpec := gen.tr.CGoSpec(spec, true)
	structSpec := spec.(*tl.CStructSpec)

	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, "func (x *%s) Ref() *%s", goStructName, cgoSpec)
	fmt.Fprintf(buf, `{
		if x == nil {
			return nil
		}
		return (*%s)(unsafe.Pointer(x))
	}`, cgoSpec)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Ref", goStructName),
		Description: "Ref returns a reference to C object as it is.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) Free()", goStructName)
	fmt.Fprint(buf, `{
		if x != nil  {
			C.free(unsafe.Pointer(x))
		}
	}`)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Free", goStructName),
		Description: "Free cleanups the referenced memory using C free.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func New%sRef(ref unsafe.Pointer) *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		return (*%s)(ref)
	}`, goStructName)
	name := fmt.Sprintf("New%sRef", goStructName)
	helpers = append(helpers, &Helper{
		Name:        name,
		Description: name + " converts the C object reference into a raw struct reference without wrapping.",
		Source:      buf.String(),
	})

	buf.Reset()
	allocHelper := gen.getAllocMemoryHelper(cgoSpec)
	fmt.Fprintf(buf, "func New%s() *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		return (*%s)(%s(1))
	}`, goStructName, allocHelper.Name)
	name = fmt.Sprintf("New%s", goStructName)
	helpers = append(helpers, &Helper{
		Name: name,
		Description: name + " allocates a new C object of this type and converts the reference into\n" +
			"a raw struct reference without wrapping.",
		Source:   buf.String(),
		Requires: []*Helper{allocHelper},
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) PassRef() *%s", goStructName, cgoSpec)
	fmt.Fprintf(buf, `{
		if x == nil {
			x = (*%s)(%s(1))
		}
		return (*%s)(unsafe.Pointer(x))
	}`, goStructName, allocHelper.Name, cgoSpec)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.PassRef", goStructName),
		Description: "PassRef returns a reference to C object as it is or allocates a new C object of this type.",
		Source:      buf.String(),
		Requires:    []*Helper{allocHelper},
	})

	if !gen.cfg.Options.StructAccessors {
		return
	}
	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
		}
		buf.Reset()
		typeName := m.Spec.GetBase()
		switch m.Spec.Kind() {
		case tl.StructKind, tl.OpaqueStructKind, tl.UnionKind, tl.EnumKind:
			if !gen.tr.IsAcceptableName(tl.TargetType, typeName) {
				continue
			}
		}
		memTip := memTipRx.TipAt(i)
		if !memTip.IsValid() {
			memTip = gen.MemTipOf(m)
		}
		ptrTip := ptrTipRx.TipAt(i)
		if memTip == tl.TipMemRaw {
			ptrTip = tl.TipPtrSRef
		}
		typeTip := typeTipRx.TipAt(i)
		goSpec := gen.tr.TranslateSpec(m.Spec, ptrTip, typeTip)
		cgoSpec := gen.tr.CGoSpec(m.Spec, false)
		// does not work for function pointers
		if goSpec.Pointers > 0 && goSpec.Base == "func" {
			continue
		}
		const public = true
		goName := string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		arr := len(goSpec.OuterArr.Sizes()) > 0 || len(goSpec.InnerArr.Sizes()) > 0
		if !arr {
			goSpec.Pointers += 1
			cgoSpec.Pointers += 1
		}
		fmt.Fprintf(buf, "func (s *%s) Get%s() %s {\n", goStructName, goName, goSpec)
		toProxy, _ := gen.proxyValueToGo(memTip, "ret", "&s."+m.Name, goSpec, cgoSpec)
		fmt.Fprintf(buf, "\tvar ret %s\n", goSpec)
		fmt.Fprintf(buf, "\t%s\n", toProxy)
		fmt.Fprintf(buf, "\treturn ret\n")
		fmt.Fprintf(buf, "}\n")
		helpers = append(helpers, &Helper{
			Name:        fmt.Sprintf("%s.Get%s", goStructName, goName),
			Description: fmt.Sprintf("Get%s returns a reference to C object within a struct", goName),
			Source:      buf.String(),
		})
	}
	return
}

func (gen *Generator) getPassRefSource(goStructName []byte, cStructName string, spec tl.CType) []byte {
	cgoSpec := gen.tr.CGoSpec(spec, false)
	structSpec := spec.(*tl.CStructSpec)
	buf := new(bytes.Buffer)
	crc := getRefCRC(spec)
	fmt.Fprintf(buf, `if x == nil {
		return nil, nil
	} else if x.ref%2x != nil {
		return x.ref%2x, nil
	}`, crc, crc)
	writeSpace(buf, 1)

	h := gen.getAllocMemoryHelper(tl.CGoSpec{Base: cgoSpec.Base})
	gen.submitHelper(h)

	fmt.Fprintf(buf, "mem%2x := %s(1)\n", crc, h.Name)
	fmt.Fprintf(buf, "ref%2x := (*%s)(mem%2x)\n", crc, cgoSpec.Base, crc)
	fmt.Fprintf(buf, "allocs%2x := new(cgoAllocMap)\n", crc)
	fmt.Fprintf(buf, "allocs%2x.Add(mem%2x)\n", crc, crc)

	writeSpace(buf, 1)

	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
			// TODO: generate setters
		}

		typeName := m.Spec.GetBase()
		switch m.Spec.Kind() {
		case tl.StructKind, tl.OpaqueStructKind, tl.UnionKind, tl.EnumKind:
			if !gen.tr.IsAcceptableName(tl.TargetType, typeName) {
				continue
			}
		}
		memTip := memTipRx.TipAt(i)
		if !memTip.IsValid() {
			memTip = gen.MemTipOf(m)
		}
		ptrTip := ptrTipRx.TipAt(i)
		if memTip == tl.TipMemRaw {
			ptrTip = tl.TipPtrSRef
		}
		typeTip := typeTipRx.TipAt(i)
		goSpec := gen.tr.TranslateSpec(m.Spec, ptrTip, typeTip)
		cgoSpec := gen.tr.CGoSpec(m.Spec, false)
		const public = true
		goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		fromProxy, nillable := gen.proxyValueFromGo(memTip, goName, goSpec, cgoSpec)
		if nillable {
			fmt.Fprintf(buf, "if %s != nil {\n", goName)
		}
		fmt.Fprintf(buf, "var c%s_allocs *cgoAllocMap\n", m.Name)
		fmt.Fprintf(buf, "ref%2x.%s, c%s_allocs  = %s\n", crc, m.Name, m.Name, fromProxy)
		fmt.Fprintf(buf, "allocs%2x.Borrow(c%s_allocs)\n", crc, m.Name)
		if nillable {
			fmt.Fprintf(buf, "}\n\n")
		} else {
			fmt.Fprint(buf, "\n")
		}
	}
	fmt.Fprintf(buf, "x.ref%2x = ref%2x\n", crc, crc)
	fmt.Fprintf(buf, "x.allocs%2x = allocs%2x\n", crc, crc)
	// auto free memory
	fmt.Fprintf(buf, "// auto free memory\n")
	fmt.Fprintf(buf, "runtime.SetFinalizer(x, free%s)\n", string(goStructName))

	fmt.Fprintf(buf, "return ref%2x, allocs%2x\n", crc, crc)
	writeSpace(buf, 1)
	return buf.Bytes()
}

func (gen *Generator) getPassValueSource(goStructName []byte, spec tl.CType) []byte {
	buf := new(bytes.Buffer)
	crc := getRefCRC(spec)
	fmt.Fprintf(buf, `if x.ref%2x != nil {
		return *x.ref%2x, nil
	}`, crc, crc)
	writeSpace(buf, 1)
	fmt.Fprintf(buf, "ref, allocs := x.PassRef()\n")
	fmt.Fprintf(buf, "return *ref, allocs\n")
	return buf.Bytes()
}

func getRefCRC(spec tl.CType) uint32 {
	return crc32.ChecksumIEEE([]byte(spec.String()))
}

func (gen *Generator) getDerefSource(goStructName []byte, cStructName string, spec tl.CType) []byte {
	structSpec := spec.(*tl.CStructSpec)
	buf := new(bytes.Buffer)
	crc := getRefCRC(spec)
	fmt.Fprintf(buf, `if x.ref%2x == nil {
		return
	}`, crc)
	writeSpace(buf, 1)

	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
			// TODO: generate getters
		}

		typeName := m.Spec.GetBase()
		switch m.Spec.Kind() {
		case tl.StructKind, tl.OpaqueStructKind, tl.UnionKind, tl.EnumKind:
			if !gen.tr.IsAcceptableName(tl.TargetType, typeName) {
				continue
			}
		}
		memTip := memTipRx.TipAt(i)
		if !memTip.IsValid() {
			memTip = gen.MemTipOf(m)
		}
		ptrTip := ptrTipRx.TipAt(i)
		if memTip == tl.TipMemRaw {
			ptrTip = tl.TipPtrSRef
		}
		typeTip := typeTipRx.TipAt(i)
		goSpec := gen.tr.TranslateSpec(m.Spec, ptrTip, typeTip)
		const public = true
		goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		cgoName := fmt.Sprintf("x.ref%2x.%s", crc, m.Name)
		cgoSpec := gen.tr.CGoSpec(m.Spec, false)
		toProxy, _ := gen.proxyValueToGo(memTip, goName, cgoName, goSpec, cgoSpec)
		fmt.Fprintln(buf, toProxy)
	}
	return buf.Bytes()
}

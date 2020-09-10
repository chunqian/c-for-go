package generator

import (
	"bytes"
	"fmt"
	"hash/crc32"

	tl "github.com/xlab/c-for-go/translator"
)

func (gen *Generator) getStructMembersHelpers(cStructName string, spec tl.CType) string {

	buf := ""
	if spec.GetPointers() > 0 {
		return "" // can't addess a pointer receiver
	}
	// cgoSpec := gen.tr.CGoSpec(spec, true)
	structSpec := spec.(*tl.CStructSpec)

	if !gen.cfg.Options.StructAccessors {
		return ""
	}
	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
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
		// cgoSpec := gen.tr.CGoSpec(m.Spec, false)
		// does not work for function pointers
		if goSpec.Pointers > 0 && goSpec.Base == "func" {
			continue
		}
		const public = true
		goName := string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		goName = unexportName(goName) + "0"
		// arr := len(goSpec.OuterArr.Sizes()) > 0 || len(goSpec.InnerArr.Sizes()) > 0
		// if !arr {
		// 	goSpec.Pointers += 1
		// 	cgoSpec.Pointers += 1
		// }

		if m.Spec.Kind() == tl.StructKind && m.Spec.GetPointers() > 0 {
			// goSpec.Raw = unexportName(goSpec.Raw)
			// goSpec.Raw = "g" + goSpec.Raw
			buf += fmt.Sprintf("%s %s,", goName, goSpec)
		} else {
			buf += fmt.Sprintf("%s %s,", goName, goSpec)
		}
		// buf += fmt.Sprintf("%s %s,", goName, goSpec)
	}
	return buf
}

func (gen *Generator) getStructHelpers(goStructName []byte, cStructName string, spec tl.CType) (helpers []*Helper) {
	crc := getRefCRC(spec)
	cgoSpec := gen.tr.CGoSpec(spec, true)
	// goStructNameL := unexportName(string(goStructName))

	buf := new(bytes.Buffer)
	// fmt.Fprintf(buf, "func clear%sMemory(x *g%s)", goStructName, goStructName)
	// fmt.Fprintf(buf, `{
	// 	if x != nil && x.allocs%2x != nil {
	// 		x.allocs%2x.(*cgoAllocMap).Free()
	// 		x.ref%2x = nil
	// 		return
	// 	}
	// }`, crc, crc, crc)
	// helpers = append(helpers, &Helper{
	// 	Name: fmt.Sprintf("free%s", goStructName),
	// 	Description: "Auto free invokes alloc map's free mechanism that cleanups any allocated memory using C free.\n" +
	// 		"Does nothing if struct is nil or has no allocation map.",
	// 	Source: buf.String(),
	// })

	// buf.Reset()
	fmt.Fprintf(buf, "func new%sRef(ref unsafe.Pointer) *g%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		if ref == nil {
			return nil
		}
		obj := new(g%s)
		obj.ref%2x = (*%s)(unsafe.Pointer(ref))
		return obj
	}`, goStructName, crc, cgoSpec)

	name := fmt.Sprintf("new%sRef", goStructName)
	helpers = append(helpers, &Helper{
		Name: name,
		Description: name + " creates a new wrapper struct with underlying reference set to the original C object.\n" +
			"Returns nil if the provided pointer to C object is nil too.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *g%s) passRef() (*%s, *cgoAllocMap) {\n", goStructName, cgoSpec)
	buf.Write(gen.getpassRefSource(goStructName, cStructName, spec))
	buf.WriteRune('}')
	helpers = append(helpers, &Helper{
		Name: fmt.Sprintf("%s.passRef", goStructName),
		Description: "passRef returns the underlying C object, otherwise it will allocate one and set its values\n" +
			"from this wrapping struct, counting allocations into an allocation map.",
		Source: buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x g%s) passValue() (%s, *cgoAllocMap) {\n", goStructName, cgoSpec)
	buf.Write(gen.getPassValueSource(goStructName, spec))
	buf.WriteRune('}')
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.passValue", goStructName),
		Description: "passValue does the same as passRef except that it will try to dereference the returned pointer.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *g%s) convert() *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
	    if x.ref%2x != nil {
	        return (*%s)(unsafe.Pointer(x.ref%2x))
	    }
	    x.passRef()
	    return (*%s)(unsafe.Pointer(x.ref%2x))
	}`, crc, goStructName, crc, goStructName, crc)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.convert", goStructName),
		Description: "convert struct for mapping C struct unanimous.",
		Source:      buf.String(),
	})

	buf.Reset()
	members := gen.getStructMembersHelpers(cStructName, spec)
	fmt.Fprintf(buf, "func New%s(%s) %s {", goStructName, members, goStructName)
	buf.Write(gen.getNewStructSource(goStructName, cStructName, spec))
	fmt.Fprintf(buf, `
        ret0, alloc0 := obj.passRef()
        if len(alloc0.m) > 0 {
            panic("Cgo memory alloced, please use func Alloc%s.")
        }
        return *(*%s)(unsafe.Pointer(ret0))
    `, goStructName, goStructName)
	buf.WriteRune('}')
	name = fmt.Sprintf("New%s", goStructName)
	helpers = append(helpers, &Helper{
		Name:        name,
		Description: name + " new Go object and Mapping to C object.",
		Source:      buf.String(),
	})

	buf.Reset()
	members = gen.getStructMembersHelpers(cStructName, spec)
	fmt.Fprintf(buf, "func Alloc%s(%s) (*%s, *cgoAllocMap) {", goStructName, members, goStructName)
	buf.Write(gen.getNewStructSource(goStructName, cStructName, spec))
	fmt.Fprintf(buf, `
        ret0, alloc0 := obj.passRef()
	    ret1 := (*%s)(unsafe.Pointer(ret0))
	    return ret1, alloc0
    `, goStructName)
	buf.WriteRune('}')
	name = fmt.Sprintf("Alloc%s", goStructName)
	helpers = append(helpers, &Helper{
		Name:        name,
		Description: name + " new Go object and Mapping to C object.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) Index(index int32) *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
	    ptr1 := (*%s)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + uintptr(index)*uintptr(sizeOf%sValue)))
	    return ptr1
	}`, goStructName, spec.GetTag())
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Index", goStructName),
		Description: "Index reads Go data structure out from plain C format.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) Index(index int32) *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
	    ptr1 := (*%s)(unsafe.Pointer(uintptr(unsafe.Pointer(x)) + uintptr(index)*uintptr(sizeOf%sValue)))
	    return ptr1
	}`, goStructName, spec.GetTag())
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.Index", goStructName),
		Description: "Index reads Go data structure out from plain C format.",
		Source:      buf.String(),
	})

	buf.Reset()
	fmt.Fprintf(buf, "func (x *%s) GC(a *cgoAllocMap)", goStructName)
	fmt.Fprintf(buf, `{
	    if len(a.m) > 0 {
	        for ptr := range a.m {
	            fmt.Printf("INFO: MEMORY: [PTR %%p] GC register\n", ptr)
	        }
	        runtime.SetFinalizer(x, func(*%s) {
	            a.Free()
	        })
	    }
	}`, goStructName)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.GC", goStructName),
		Description: "GC is register for garbage collection.",
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

		// Get* func
		switch {
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 0 && goSpec.Pointers == 0 && len(goSpec.OuterArr.Sizes()) == 1:
			goSpecS0P0Out0 := goSpec
			goSpecS0P0Out0.OuterArr = ""

			fmt.Fprintf(buf, "func (x *%s) %ser(index int32) *%s", goStructName, goName, goSpecS0P0Out0)
			fmt.Fprintf(buf, `{
				var ret *%s

				ptr0 := &x.%s
				ptr1 := (*%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(index)*uintptr(sizeOf%sValue)))
				ret = new%sRef(unsafe.Pointer(ptr1)).convert()

				return ret
			}`, goSpecS0P0Out0, goName, cgoSpec.Base, goSpecS0P0Out0, goSpecS0P0Out0)
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 1:
			goSpecS0P0 := goSpec
			goSpecS0P0.Slices -= 1
			goSpecS0P0.Pointers -= 1

			fmt.Fprintf(buf, "func (x *%s) %ser(index int32) *%s", goStructName, goName, goSpecS0P0)
			fmt.Fprintf(buf, `{
				var ret *%s

				ptr0 := x.%s
				ptr1 := (*%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(index)*uintptr(sizeOf%sValue)))
				ret = new%sRef(unsafe.Pointer(ptr1)).convert()

				return ret
			}`, goSpecS0P0, goName, cgoSpec.Base, goSpecS0P0, goSpecS0P0)
		case goSpec.Kind == tl.StructKind && goSpec.Slices == 2:
			goSpecS0P0 := goSpec
			goSpecS0P0.Pointers -= 1
			goSpecS0P0.Slices -= 2

			cgoSpecP2 := cgoSpec
			cgoSpecP2.Pointers -= 1

			cgoSpecP1 := cgoSpec
			cgoSpecP1.Pointers -= 2

			cgoSpecP0 := cgoSpec
			cgoSpecP0.Pointers -= 3
			fmt.Fprintf(buf, "func (x *%s) %ser(row int32, column int32) *%s", goStructName, goName, goSpecS0P0)
			fmt.Fprintf(buf, `{
				var ret *%s

				ptr0 := x.%s
				ptr1 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(row)*uintptr(sizeOfPtr)))
				ptr2 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(*ptr1)) + uintptr(column)*uintptr(sizeOf%sValue)))
				ret = new%sRef(unsafe.Pointer(ptr2)).convert()

				return ret
			}`, goSpecS0P0, goName, cgoSpecP2, cgoSpecP1, m.Spec.GetBase(), m.Spec.GetBase())
		case goSpec.Kind == tl.PlainTypeKind && goSpec.Slices > 0:
			goSpecS0 := goSpec
			goSpecS0.Slices = 0

			fmt.Fprintf(buf, "func (x *%s) %ser(index int32) %s {\n", goStructName, goName, goSpecS0)
			toProxy, _ := gen.proxyValueToGo(memTip, "ret", goName, goSpec, cgoSpec)
			fmt.Fprintf(buf, "\tvar ret %s\n", goSpecS0)
			fmt.Fprintf(buf, "\t%s\n", toProxy)
			fmt.Fprintf(buf, "\treturn ret\n")
			fmt.Fprintf(buf, "}\n")
		}

		if len(goSpec.OuterArr.Sizes()) == 1 || goSpec.Slices > 0 {
			helpers = append(helpers, &Helper{
				Name:        fmt.Sprintf("%s.%s", goStructName, goName),
				Description: fmt.Sprintf("%s returns a reference to C object within a struct", goName),
				Source:      buf.String(),
			})
			buf.Reset()
		}

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
	fmt.Fprintf(buf, "func new%sRef(ref unsafe.Pointer) *%s", goStructName, goStructName)
	fmt.Fprintf(buf, `{
		return (*%s)(ref)
	}`, goStructName)
	name := fmt.Sprintf("new%sRef", goStructName)
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
	fmt.Fprintf(buf, "func (x *%s) passRef() *%s", goStructName, cgoSpec)
	fmt.Fprintf(buf, `{
		if x == nil {
			x = (*%s)(%s(1))
		}
		return (*%s)(unsafe.Pointer(x))
	}`, goStructName, allocHelper.Name, cgoSpec)
	helpers = append(helpers, &Helper{
		Name:        fmt.Sprintf("%s.passRef", goStructName),
		Description: "passRef returns a reference to C object as it is or allocates a new C object of this type.",
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

func (gen *Generator) getpassRefSource(goStructName []byte, cStructName string, spec tl.CType) []byte {
	cgoSpec := gen.tr.CGoSpec(spec, false)
	structSpec := spec.(*tl.CStructSpec)
	buf := new(bytes.Buffer)
	crc := getRefCRC(spec)
	fmt.Fprintf(buf, `if x == nil {
		return nil, nil
	} else if x.ref%2x != nil {
		if x.allocs%2x != nil {
			return x.ref%2x, x.allocs%2x.(*cgoAllocMap)
		} else {
			return x.ref%2x, nil
		}
	}`, crc, crc, crc, crc, crc)
	writeSpace(buf, 1)

	h := gen.getAllocMemoryHelper(tl.CGoSpec{Base: cgoSpec.Base})
	gen.submitHelper(h)

	// fmt.Fprintf(buf, "mem%2x := %s(1)\n", crc, h.Name)
	fmt.Fprintf(buf, "mem%2x := unsafe.Pointer(new(%s))\n", crc, cgoSpec.Base)
	fmt.Fprintf(buf, "ref%2x := (*%s)(mem%2x)\n", crc, cgoSpec.Base, crc)
	fmt.Fprintf(buf, "allocs%2x := new(cgoAllocMap)\n", crc)
	fmt.Fprintf(buf, "// allocs%2x.Add(mem%2x)\n", crc, crc)

	// fmt.Fprintf(buf, `defer func() {
	// 	if len(x.allocs%2x.(*cgoAllocMap).m) > 0 {
	// 		runtime.SetFinalizer(x, free%s)
	// 	}
	// }()`, crc, string(goStructName))

	// writeSpace(buf, 2)

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
		// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		goName := "x." + "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		fromProxy, nillable := gen.proxyValueFromGo(memTip, goName, goSpec, cgoSpec)
		if nillable {
			fmt.Fprintf(buf, "if %s != nil {\n", goName)
		}
		fmt.Fprintf(buf, "var c%s_allocs *cgoAllocMap\n", m.Name)
		fmt.Fprintf(buf, "ref%2x.%s, c%s_allocs  = %s\n", crc, m.Name, m.Name, fromProxy)
		fmt.Fprintf(buf, "allocs%2x.Borrow(c%s_allocs)\n", crc, m.Name)
		// reset

		if goSpec.Kind == tl.StructKind {
			goSpec.Raw = "g" + goSpec.Raw
		}

		fmt.Fprintf(buf, "%s = *new(%s)\n", goName, goSpec)
		if nillable {
			fmt.Fprintf(buf, "}\n\n")
		} else {
			fmt.Fprint(buf, "\n")
		}
	}
	fmt.Fprintf(buf, "x.ref%2x = ref%2x\n", crc, crc)
	fmt.Fprintf(buf, "x.allocs%2x = allocs%2x\n", crc, crc)

	writeSpace(buf, 1)
	fmt.Fprintf(buf, "return ref%2x, allocs%2x\n", crc, crc)
	// writeSpace(buf, 1)
	return buf.Bytes()
}

func (gen *Generator) getNewStructSource(goStructName []byte, cStructName string, spec tl.CType) []byte {
	// cgoSpec := gen.tr.CGoSpec(spec, false)
	structSpec := spec.(*tl.CStructSpec)
	// goStructNameL := unexportName(string(goStructName))

	buf := new(bytes.Buffer)
	// crc := getRefCRC(spec)
	fmt.Fprintf(buf, "obj := *new(g%s)\n", goStructName)

	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeType, cStructName, spec)
	for i, m := range structSpec.Members {
		if len(m.Name) == 0 {
			continue
			// TODO: generate setters
		}

		typeName := m.Spec.GetBase()
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

		switch m.Spec.Kind() {
		case tl.StructKind, tl.OpaqueStructKind, tl.UnionKind, tl.EnumKind:
			if !gen.tr.IsAcceptableName(tl.TargetType, typeName) {
				continue
			}
		}

		switch {
		case m.Spec.Kind() == tl.StructKind && m.Spec.GetPointers() == 0 && len(goSpec.OuterArr.Sizes()) == 0:
			const public = true
			// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goElementName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "0"

			fmt.Fprintf(buf, "obj.%s  = *new%sRef(unsafe.Pointer(&%s))\n", goName, typeName, goElementName)
		case m.Spec.Kind() == tl.StructKind && m.Spec.GetPointers() == 1 && len(goSpec.OuterArr.Sizes()) == 0:
			const public = true
			// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goElementName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "0"
			goTmpName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "1"

			goSpec.Raw = "g" + goSpec.Raw
			fmt.Fprintf(buf, `
				var %s %s
				for i0 := range %s {
					p0 := *new%sRef(unsafe.Pointer(&%s[i0]))
					%s = append(%s, p0)
				}
				obj.%s = %s
         	`, goTmpName, goSpec, goElementName, typeName, goElementName, goTmpName, goTmpName, goName, goTmpName)
		case m.Spec.Kind() == tl.StructKind && m.Spec.GetPointers() == 2 && len(goSpec.OuterArr.Sizes()) == 0:
			const public = true
			// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goElementName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "0"
			goTmpName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "1"

			goSpec.Raw = "g" + goSpec.Raw

			goSpecS1 := goSpec
			goSpecS1.Slices -= 1

			fmt.Fprintf(buf, `
				var %s %s
				for i0, v0 := range %s {
					s0 := make(%s, len(v0))
					for i1 := range v0 {
						p0 := *new%sRef(unsafe.Pointer(&%s[i0][i1]))
						s0 = append(s0, p0)
					}
					%s = append(%s, s0)
				}
				obj.%s = %s
         	`, goTmpName, goSpec, goElementName, goSpecS1, typeName, goElementName, goTmpName, goTmpName, goName, goTmpName)
		case m.Spec.Kind() == tl.StructKind && m.Spec.GetPointers() == 0 && len(goSpec.OuterArr.Sizes()) == 1:
			const public = true
			// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goElementName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "0"
			goTmpName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "1"

			goSpec.Raw = "g" + goSpec.Raw
			fmt.Fprintf(buf, `
				var %s %s
				for i0 := range %s {
					p0 := *new%sRef(unsafe.Pointer(&%s[i0]))
					%s[i0] = p0
				}
				obj.%s = %s
         	`, goTmpName, goSpec, goElementName, typeName, goElementName, goTmpName, goName, goTmpName)
		default:
			const public = true
			// goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
			goElementName := unexportName(string(gen.tr.TransformName(tl.TargetType, m.Name, public))) + "0"

			fmt.Fprintf(buf, "obj.%s  = %s\n", goName, goElementName)
		}

		// const public = true
		// // goName := "x." + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		// goName := "g" + string(gen.tr.TransformName(tl.TargetType, m.Name, public))
		// fmt.Fprintf(buf, "obj.%s  = %s\n", goName, goName)
	}

	// fmt.Fprintf(buf, "obj.passRef()\n")
	// fmt.Fprintf(buf, "return obj\n")
	return buf.Bytes()
}

func (gen *Generator) getPassValueSource(goStructName []byte, spec tl.CType) []byte {
	buf := new(bytes.Buffer)
	crc := getRefCRC(spec)
	fmt.Fprintf(buf, `if x.ref%2x != nil {
		return *x.ref%2x, nil
	}`, crc, crc)
	writeSpace(buf, 1)
	fmt.Fprintf(buf, "ref, allocs := x.passRef()\n")
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

package generator

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	tl "github.com/xlab/c-for-go/translator"
)

type HelperSide string

const (
	NoSide HelperSide = ""
	GoSide HelperSide = "go"
	CHSide HelperSide = "h"
	CCSide HelperSide = "c"
)

type Helpers []*Helper

type Helper struct {
	Name        string
	Side        HelperSide
	Description string
	Source      string
	Nillable    bool
	Requires    []*Helper
}

type getHelperFunc func(gen *Generator, spec tl.CGoSpec) *Helper

var fromGoHelperMap = map[tl.GoTypeSpec]getHelperFunc{
	tl.StringSpec:  (*Generator).getUnpackStringHelper,
	tl.UStringSpec: (*Generator).getUnpackStringHelper,
}
var fromGoHelperMapEx = map[tl.GoTypeSpec]getHelperFunc{
	tl.StringSpec: (*Generator).getUnpackMemoryStringHelper,
}

var toGoHelperMap = map[tl.GoTypeSpec]getHelperFunc{
	tl.StringSpec:  (*Generator).getPackStringHelper,
	tl.UStringSpec: (*Generator).getPackStringHelper,
}

type proxyDecl struct {
	Name string
	Decl string
}

// getHelperName transforms signatures like [4][4][]*[4]string into A4A4SPA4String
// suitable for the generated proxy helper methods.
func getHelperName(goSpec tl.GoTypeSpec) string {
	buf := new(bytes.Buffer)
	for _, size := range goSpec.OuterArr.Sizes() {
		if len(size.Str) > 0 {
			fmt.Fprintf(buf, "AX")
		}
		fmt.Fprintf(buf, "A%d", size.N)
	}
	buf.WriteString(strings.Repeat("S", int(goSpec.Slices)))
	buf.WriteString(strings.Repeat("P", int(goSpec.Pointers)))
	for _, size := range goSpec.InnerArr.Sizes() {
		if len(size.Str) > 0 {
			fmt.Fprintf(buf, "AX")
		}
		fmt.Fprintf(buf, "A%d", size.N)
	}
	if goSpec.Unsigned {
		buf.WriteRune('U')
	}
	buf.WriteString(strings.Title(goSpec.PlainType()))
	return buf.String()
}

func (gen *Generator) getTypedHelperName(goBase string, spec tl.CGoSpec) string {
	if strings.HasPrefix(spec.Base, "C.") {
		spec.Base = spec.Base[2:]
	}
	specName := gen.tr.TransformName(tl.TargetType, spec.Base)
	buf := new(bytes.Buffer)
	for _, size := range spec.OuterArr.Sizes() {
		if len(size.Str) > 0 {
			fmt.Fprintf(buf, "AX")
		}
		fmt.Fprintf(buf, "A%d", size.N)
	}
	buf.WriteString(strings.Repeat("P", int(spec.Pointers)))
	for _, size := range spec.InnerArr.Sizes() {
		if len(size.Str) > 0 {
			fmt.Fprintf(buf, "AX")
		}
		fmt.Fprintf(buf, "A%d", size.N)
	}
	buf.Write(bytes.Title(specName))
	buf.WriteString(strings.Title(goBase))
	return buf.String()
}

type level struct {
	Array bool
	Slice bool
	Last  bool
}

type reverseBuffer []string

func (r *reverseBuffer) Linef(f string, a ...interface{}) {
	if r == nil {
		*r = make(reverseBuffer, 0, 64)
	}
	*r = append(*r, fmt.Sprintf(f, a...))
}

func (r *reverseBuffer) Line(a ...interface{}) {
	if r == nil {
		*r = make(reverseBuffer, 0, 64)
	}
	*r = append(*r, fmt.Sprint(a...))
}

func (r reverseBuffer) String() string {
	buf := new(bytes.Buffer)
	for i := len(r) - 1; i >= 0; i-- {
		buf.WriteString(r[i])
	}
	return buf.String()
}
func (r reverseBuffer) WriteTo(w io.Writer) (n int64, err error) {
	buf := new(bytes.Buffer)
	for i := len(r) - 1; i >= 0; i-- {
		buf.WriteString(r[i])
	}
	n, _ = buf.WriteTo(w)
	return n, nil
}

func ptrs(n uint8) string {
	return strings.Repeat("*", int(n))
}

func genIndices(name string, level uint8) []byte {
	buf := new(bytes.Buffer)
	for i := uint8(0); i < level; i++ {
		fmt.Fprintf(buf, "[i%d]", i)
	}
	return buf.Bytes()
}

func unpackPlain(buf io.Writer, goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec, level uint8) {
	uplevel := level - 1
	if goSpec.Pointers > 0 {
		fmt.Fprintf(buf, "v%d[i%d] = (%s)(unsafe.Pointer(x%d))\n",
			uplevel, uplevel, cgoSpec.AtLevel(level), level)
		return
	} else if len(goSpec.OuterArr) > 0 {
		fmt.Fprintf(buf, "v%d[i%d] = *(*%s)(unsafe.Pointer(x%d))\n",
			uplevel, uplevel, cgoSpec.AtLevel(level), level)
		return
	}
	fmt.Fprintf(buf, "v%d[i%d] = *(*%s)(unsafe.Pointer(&x%d))\n",
		uplevel, uplevel, cgoSpec.Base, level)
}

func unpackPlainSlice(buf io.Writer, cgoSpec tl.CGoSpec, level uint8) {
	uplevel := level - 1
	fmt.Fprintf(buf, "h := (*sliceHeader)(unsafe.Pointer(&x%s))\n", genIndices("i", level))
	fmt.Fprintf(buf, "v%d[i%d] = (%s)(h.Data)\n",
		uplevel, uplevel, cgoSpec.AtLevel(level))
}

func (gen *Generator) unpackObj(buf io.Writer, goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec, level uint8) *Helper {
	uplevel := level - 1
	indices := genIndices("i", level)
	if getHelper, ok := fromGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		fmt.Fprintf(buf, "v%d[i%d], _ = %s(%sx%s)\n",
			uplevel, uplevel, helper.Name, ptrs(goSpec.Pointers), indices)
		return helper
	}
	if goSpec.Pointers == 0 {
		fmt.Fprintf(buf, "allocs%d := new(cgoAllocMap)\n", uplevel)
		fmt.Fprintf(buf, "v%d[i%d], allocs%d = x%s.passValue()\n", uplevel, uplevel, uplevel, indices)
		fmt.Fprintf(buf, "allocs.Borrow(allocs%d)\n", uplevel)
		return nil
	}
	fmt.Fprintf(buf, "v%d[i%d], _ = x%s.passRef()\n", uplevel, uplevel, indices)
	return nil
}

func (gen *Generator) unpackObjEx(buf io.Writer, goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec, level uint8) *Helper {
	uplevel := level - 1
	indices := genIndices("i", level)
	if getHelper, ok := fromGoHelperMapEx[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		fmt.Fprintf(buf, "var allocs%d *cgoAllocMap\n", uplevel)
		fmt.Fprintf(buf, "v%d[i%d], allocs%d = %s(%sx%s)\n",
			uplevel, uplevel, uplevel, helper.Name, ptrs(goSpec.Pointers), indices)
		fmt.Fprintf(buf, "allocs.Borrow(allocs%d)\n", uplevel)
		return helper
	}
	isPlain := goSpec.IsPlain() || goSpec.IsPlainKind()

	if isPlain {
		cgoSpecP1 := cgoSpec
		cgoSpecP1.Pointers -= 1
		if cgoSpecP1.Pointers > 0 {
			fmt.Fprintf(buf, "v%d[i%d] = *(%s)(unsafe.Pointer(&x%s))\n", uplevel, uplevel, cgoSpecP1, indices)
		} else {
			fmt.Fprintf(buf, "v%d[i%d] = (%s)(x%s)\n", uplevel, uplevel, cgoSpecP1, indices)
		}

		return nil
	}
	if goSpec.Pointers == 0 {
		fmt.Fprintf(buf, "allocs%d := new(cgoAllocMap)\n", uplevel)
		fmt.Fprintf(buf, "v%d[i%d], allocs%d = x%s.passValue()\n", uplevel, uplevel, uplevel, indices)
		fmt.Fprintf(buf, "allocs.Borrow(allocs%d)\n", uplevel)
		return nil
	}
	fmt.Fprintf(buf, "v%d[i%d], _ = x%s.passRef()\n", uplevel, uplevel, indices)
	return nil
}

func cgoSpecArg(cgoSpec tl.CGoSpec, level uint8, isArg bool) string {
	if isArg {
		return cgoSpec.AtLevel(level)
	} else if level == 0 {
		return cgoSpec.String()
	}
	return cgoSpec.AtLevel(level)
}

func (gen *Generator) unpackArray(buf1 io.Writer, buf2 *reverseBuffer, cgoSpec tl.CGoSpec, level uint8, isArg bool) {
	uplevel := level - 1
	if level == 0 {
		h := gen.getAllocMemoryHelper(cgoSpec)
		gen.submitHelper(h)
		gen.submitHelper(sizeOfPtr)
		gen.submitHelper(cgoAllocMap)

		fmt.Fprintf(buf1, `allocs = new(cgoAllocMap)
		defer runtime.SetFinalizer(&unpacked, func(*%s) {
			go allocs.Free()
		})`, cgoSpec)
		fmt.Fprintf(buf1, "\n\nmem0 := %s(1)\n", h.Name)
		fmt.Fprintf(buf1, "allocs.Add(mem0)\n")
		fmt.Fprintf(buf1, "v0 := (*%s)(mem0)\n", cgoSpec)
		fmt.Fprintf(buf1, "for i0 := range x {\n")
		buf2.Linef("return\n")
		if isArg {
			buf2.Linef("unpacked = (%s)(mem0)\n", cgoSpec.AtLevel(0))
		} else {
			buf2.Linef("unpacked = *(*%s)(mem0)\n", cgoSpec.String())
		}
		buf2.Linef("}\n")
		return
	}

	h := gen.getAllocMemoryHelper(cgoSpec.SpecAtLevel(level))
	gen.submitHelper(h)
	gen.submitHelper(sizeOfPtr)
	gen.submitHelper(cgoAllocMap)
	fmt.Fprintf(buf1, "mem%d := %s(1)\n", level, h.Name)
	fmt.Fprintf(buf1, "allocs.Add(mem%d)\n", level)
	fmt.Fprintf(buf1, "v%d := (*%s)(mem%d)\n", level, cgoSpec.AtLevel(level), level)
	fmt.Fprintf(buf1, "for i%d := range x%s {\n", level, genIndices("i", level))
	buf2.Linef("v%d[i%d] = *(*%s)(mem%d)\n",
		uplevel, uplevel, cgoSpec.AtLevel(level), level)
	buf2.Linef("}\n")
}

func notNilBarrier(buf io.Writer, name string) {
	fmt.Fprintf(buf, `if %s == nil {
		return nil, nil
	}`, name)
}

func (gen *Generator) unpackSlice(buf1 io.Writer, buf2 *reverseBuffer, cgoSpec tl.CGoSpec, level uint8, isArg bool) {
	uplevel := level - 1

	if level == 0 {
		notNilBarrier(buf1, "x")
		writeSpace(buf1, 1)

		levelSpec := cgoSpec.SpecAtLevel(1)
		h := gen.getAllocMemoryHelper(levelSpec)
		gen.submitHelper(h)
		gen.submitHelper(sizeOfPtr)
		gen.submitHelper(cgoAllocMap)

		fmt.Fprintf(buf1, `allocs = new(cgoAllocMap)
		defer runtime.SetFinalizer(&unpacked, func(*%s) {
			go allocs.Free()
		})`, cgoSpec)
		fmt.Fprintf(buf1, "\n\nlen0 := len(x)\n")
		fmt.Fprintf(buf1, "mem0 := %s(len0)\n", h.Name)
		fmt.Fprintf(buf1, "allocs.Add(mem0)\n")
		fmt.Fprintf(buf1, `h0 := &sliceHeader{
			Data: mem0,
			Cap: len0,
			Len: len0,
		}`)
		fmt.Fprintf(buf1, "\nv0 := *(*[]%s)(unsafe.Pointer(h0))\n", levelSpec)
		fmt.Fprintf(buf1, "for i0 := range x {\n")

		buf2.Linef("return\n")
		buf2.Linef("unpacked = (%s)(h.Data)\n", cgoSpecArg(cgoSpec, 0, isArg))
		buf2.Linef("h := (*sliceHeader)(unsafe.Pointer(&v0))\n")
		buf2.Linef("}\n")
		return
	}
	indices := genIndices("i", level)

	levelSpec := cgoSpec.SpecAtLevel(level + 1)
	h := gen.getAllocMemoryHelper(levelSpec)
	gen.submitHelper(h)
	gen.submitHelper(sizeOfPtr)
	fmt.Fprintf(buf1, "len%d := len(x%s)\n", level, indices)
	fmt.Fprintf(buf1, "mem%d := %s(len%d)\n", level, h.Name, level)
	fmt.Fprintf(buf1, "allocs.Add(mem%d)\n", level)
	fmt.Fprintf(buf1, `h%d := &sliceHeader{
			Data: mem%d,
			Cap: len%d,
			Len: len%d,
		}`, level, level, level, level)
	fmt.Fprintf(buf1, "\nv%d := *(*[]%s)(unsafe.Pointer(h%d))\n", level, levelSpec, level)

	fmt.Fprintf(buf1, "for i%d := range x%s {\n", level, indices)
	buf2.Linef("v%d[i%d] = (%s)(h.Data)\n", uplevel, uplevel, cgoSpec.AtLevel(level))
	buf2.Linef("h := (*sliceHeader)(unsafe.Pointer(&v%d))\n", level)
	buf2.Linef("}\n")
}

func (gen *Generator) getAllocMemoryHelper(cgoSpec tl.CGoSpec) *Helper {
	name := "alloc" + gen.getTypedHelperName("memory", cgoSpec)
	sizeofConst := "sizeOf" + gen.getTypedHelperName("value", cgoSpec)
	helper := &Helper{
		Name: name,
		Description: fmt.Sprintf(`%s allocates memory for type %s in C.
The caller is responsible for freeing the this memory via C.free.`, name, cgoSpec),
		Requires: []*Helper{cgoAllocMap},
	}

	buf := new(bytes.Buffer)
	fmt.Fprintf(buf, `func %s(n int) unsafe.Pointer {
			mem, err := C.calloc(C.size_t(n), (C.size_t)(%s))
			if err != nil {
				panic("memory alloc error: " + err.Error())
			}
			return mem
		}`, name, sizeofConst)
	fmt.Fprintln(buf)
	fmt.Fprintf(buf, `const %s = unsafe.Sizeof([1]%s{})`, sizeofConst, cgoSpec)

	helper.Source = buf.String()
	return helper
}

func (gen *Generator) getUnpackStringHelper(cgoSpec tl.CGoSpec) *Helper {
	cgoSpec = tl.CGoSpec{
		Pointers: 1,
		Base:     cgoSpec.Base,
	}
	name := "unpack" + gen.getTypedHelperName("string", cgoSpec)
	if gen.cfg.Options.SafeStrings {
		return &Helper{
			Name:        name,
			Description: fmt.Sprintf("%s represents the data from Go string as %s and avoids copying.", name, cgoSpec),
			Source: fmt.Sprintf(`func %s(str string) (%s, *cgoAllocMap) {
			str = safeString(str)
			h := (*stringHeader)(unsafe.Pointer(&str))
			return (%s)(h.Data), cgoAllocsUnknown
		}`, name, cgoSpec, cgoSpec),
			Requires: []*Helper{stringHeader, cgoAllocMap, safeString},
		}
	}
	return &Helper{
		Name:        name,
		Description: fmt.Sprintf("%s represents the data from Go string as %s and avoids copying.", name, cgoSpec),
		Source: fmt.Sprintf(`func %s(str string) (%s, *cgoAllocMap) {
			h := (*stringHeader)(unsafe.Pointer(&str))
			return (%s)(h.Data), cgoAllocsUnknown
		}`, name, cgoSpec, cgoSpec),
		Requires: []*Helper{stringHeader, cgoAllocMap},
	}
}

func (gen *Generator) getUnpackMemoryStringHelper(cgoSpec tl.CGoSpec) *Helper {
	cgoSpec = tl.CGoSpec{
		Pointers: 1,
		Base:     cgoSpec.Base,
	}
	name := "unpackMemory" + gen.getTypedHelperName("string", cgoSpec)
	return &Helper{
		Name:        name,
		Description: fmt.Sprintf("%s represents the data from Go string as %s and avoids copying.", name, cgoSpec),
		Source: fmt.Sprintf(`func %s(str string) (%s, *cgoAllocMap) {
			ptr0 := C.CString(str)
			mem0 := unsafe.Pointer(ptr0)
			allocs0 := new(cgoAllocMap)
			allocs0.Add(mem0)
			return ptr0, allocs0
		}`, name, cgoSpec),
		Requires: []*Helper{stringHeader, cgoAllocMap},
	}
}

func goSpecArg(goSpec tl.GoTypeSpec, isArg bool) string {
	if goSpec.Kind == tl.StructKind {
		goSpec.Raw = "g" + goSpec.Raw
	}

	if !isArg {
		return goSpec.String()
	}
	if len(goSpec.OuterArr) > 0 {
		return "*" + goSpec.String()
	}
	return goSpec.String()
}

func (gen *Generator) getUnpackHelper(goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec, isArg bool) *Helper {
	name := "unpack"
	if isArg {
		name += "Arg" + getHelperName(goSpec)
	} else {
		name += getHelperName(goSpec)
	}
	code := new(bytes.Buffer)
	fmt.Fprintf(code, "func %s(x %s) (unpacked %s, allocs *cgoAllocMap) {\n",
		name, goSpecArg(goSpec, isArg), cgoSpecArg(cgoSpec, 0, isArg))
	h := &Helper{
		Name:        name,
		Description: fmt.Sprintf("%s transforms a sliced Go data structure into plain C format.", name),
	}
	var level uint8
	buf1 := new(bytes.Buffer)
	buf2 := new(reverseBuffer)

	for range goSpec.OuterArr.Sizes() {
		gen.unpackArray(buf1, buf2, cgoSpec, level, isArg)
		level++
	}
	goSpec.OuterArr = ""

	for goSpec.Slices > 1 {
		goSpec.Slices--
		gen.submitHelper(sliceHeader)
		gen.unpackSlice(buf1, buf2, cgoSpec, level, isArg)
		level++
	}
	isSlice := goSpec.Slices > 0
	isPlain := goSpec.IsPlain() || goSpec.IsPlainKind()

	switch {
	// case isPlain && isSlice:
	// 	gen.submitHelper(sliceHeader)
	// 	unpackPlainSlice(buf1, cgoSpec, level)
	case isPlain && isSlice:
		gen.submitHelper(sliceHeader)
		gen.unpackSlice(buf1, buf2, cgoSpec, level, isArg)
		goSpec.Slices = 0
		if helper := gen.unpackObjEx(buf1, goSpec, cgoSpec, level+1); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	case isPlain:
		unpackPlain(buf1, goSpec, cgoSpec, level)
	case isSlice && cgoSpec.Base == "C.char" && cgoSpec.Pointers == 2:
		gen.submitHelper(sliceHeader)
		gen.unpackSlice(buf1, buf2, cgoSpec, level, isArg)
		goSpec.Slices = 0
		if helper := gen.unpackObjEx(buf1, goSpec, cgoSpec, level+1); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	case isSlice:
		gen.submitHelper(sliceHeader)
		gen.unpackSlice(buf1, buf2, cgoSpec, level, isArg)
		goSpec.Slices = 0
		if helper := gen.unpackObj(buf1, goSpec, cgoSpec, level+1); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	default:
		if helper := gen.unpackObj(buf1, goSpec, cgoSpec, level); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	}

	buf1.WriteTo(code)
	buf2.WriteTo(code)
	fmt.Fprintln(code, "}")
	h.Source = code.String()
	return h
}

func (gen *Generator) proxyValueFromGo(memTip tl.Tip, name string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {

	if getHelper, ok := fromGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	}
	gen.submitHelper(cgoAllocMap)

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
		isPlain && goSpec.Slices > 1:                             // ex: [][]byte
		helper := gen.getUnpackHelper(goSpec, cgoSpec, false)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	// case isPlain && goSpec.Slices != 0: // ex: []byte
	// 	gen.submitHelper(sliceHeader)
	// 	proxy = fmt.Sprintf(
	// 		"(%s)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&%s)).Data)), cgoAllocsUnknown",
	// 		cgoSpec, name)
	// 	return
	case isPlain && goSpec.Slices != 0: // ex: []byte
		helper := gen.getUnpackHelper(goSpec, cgoSpec, false)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	case isPlain: // ex: byte, [4]byte
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("(%s)(%s), cgoAllocsUnknown", cgoSpec, name)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			var ref string
			if goSpec.Pointers == 0 {
				ref = "&"
			}
			proxy = fmt.Sprintf("(*[0]byte)(unsafe.Pointer(%s%s)), cgoAllocsUnknown", ref, name)
			return
		}
		proxy = fmt.Sprintf("*(*%s)(unsafe.Pointer(&%s)), cgoAllocsUnknown", cgoSpec, name)
		return
	default: // ex: *SomeType
		if goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("%s.passValue()", name)
			return
		}
		proxy = fmt.Sprintf("%s.passRef()", name)
		return
	}
}

func (gen *Generator) proxyValueFromGoEx(memTip tl.Tip, name string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {

	if getHelper, ok := fromGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	}
	gen.submitHelper(cgoAllocMap)

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
		isPlain && goSpec.Slices > 1:                             // ex: [][]byte
		helper := gen.getUnpackHelper(goSpec, cgoSpec, false)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	case isPlain && goSpec.Slices != 0: // ex: []byte
		gen.submitHelper(sliceHeader)
		proxy = fmt.Sprintf(
			"(%s)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&%s)).Data))",
			cgoSpec, name)
		return
	case isPlain: // ex: byte, [4]byte
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("(%s)(%s)", cgoSpec, name)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			var ref string
			if goSpec.Pointers == 0 {
				ref = "&"
			}
			proxy = fmt.Sprintf("(*[0]byte)(unsafe.Pointer(%s%s))", ref, name)
			return
		}
		proxy = fmt.Sprintf("*(*%s)(unsafe.Pointer(&%s))", cgoSpec, name)
		return
	default: // ex: *SomeType
		if goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("%s.passValue()", name)
			return
		}
		proxy = fmt.Sprintf("%s.passRef()", name)
		return
	}
}

func (gen *Generator) proxyArgFromGo(memTip tl.Tip, name string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {

	if goSpec.IsGoString() {
		helper := gen.getUnpackStringHelper(cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	} else if getHelper, ok := fromGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	}
	gen.submitHelper(cgoAllocMap)

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
		isPlain && goSpec.Slices > 1:                             // ex: [][]byte
		helper := gen.getUnpackHelper(goSpec, cgoSpec, true)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s(%s)", helper.Name, name)
		return proxy, helper.Nillable
	case isPlain && goSpec.Slices != 0: // ex: []byte
		gen.submitHelper(sliceHeader)
		if goSpec.Base == "unsafe.Pointer" &&
			(len(goSpec.Raw) == 0 || goSpec.Raw == "unsafe.Pointer") {
			// Go 1.8+
			cgoSpec.Base = "unsafe.Pointer"
		}
		proxy = fmt.Sprintf(
			"(%s)(unsafe.Pointer((*sliceHeader)(unsafe.Pointer(&%s)).Data)), cgoAllocsUnknown",
			cgoSpec.AtLevel(0), name)
		return
	case isPlain: // ex: byte, [4]byte
		var ref, ptr string
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("(%s)(%s), cgoAllocsUnknown", cgoSpec.AtLevel(0), name)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			var ref string
			if goSpec.Pointers == 0 {
				ref = "&"
			}
			proxy = fmt.Sprintf("*(**[0]byte)(unsafe.Pointer(%s%s)), cgoAllocsUnknown", ref, name)
			return
		} else if goSpec.Base == "unsafe.Pointer" &&
			(len(goSpec.Raw) == 0 || goSpec.Raw == "unsafe.Pointer") {
			// Go 1.8+
			proxy = fmt.Sprintf("%s, cgoAllocsUnknown", name)
			return
		} else if goSpec.Pointers == 0 {
			ref = "&"
			ptr = "*"
		}
		proxy = fmt.Sprintf("%s(%s%s)(unsafe.Pointer(%s%s)), cgoAllocsUnknown",
			ptr, ptr, cgoSpec.AtLevel(0), ref, name)
		return
	default: // ex: *SomeType
		if goSpec.Pointers == 0 {
			// proxy = fmt.Sprintf("%s.passValue()", name)
			proxy = fmt.Sprintf("*(*%s)(unsafe.Pointer(&%s)), cgoAllocsUnknown", cgoSpec, name)
			return
		}
		// proxy = fmt.Sprintf("%s.passRef()", name)
		cgoSpec.Base = "C." + goSpec.Raw
		proxy = fmt.Sprintf("(%s)(unsafe.Pointer(%s)), cgoAllocsUnknown", cgoSpec, name)
		return
	}
}

func packPlain(buf io.Writer, cgoSpec tl.CGoSpec, base string, pointers uint8, level uint8) {
	uplevel := level - 1
	fmt.Fprintf(buf, "v%s = (%s%s)(unsafe.Pointer(%sptr%d))\n",
		genIndices("i", level), ptrs(pointers-uplevel), base, ptrs(cgoSpec.Pointers-level), level)
}

func (gen *Generator) packPlainSlice(buf io.Writer, base string, pointers uint8, level uint8) {
	postfix := gen.randPostfix()
	fmt.Fprintf(buf, "hx%2x := (*sliceHeader)(unsafe.Pointer(&v%s))\n", postfix, genIndices("i", level))
	fmt.Fprintf(buf, "hx%2x.Data = unsafe.Pointer(ptr%d)\n", postfix, level)
	fmt.Fprintf(buf, "hx%2x.Cap = %s\n", postfix, gen.maxMem)
	fmt.Fprintf(buf, "// hx%2x.Len = ?\n", postfix)
}

func (gen *Generator) packObj(buf io.Writer, goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec, level uint8) *Helper {
	if getHelper, ok := toGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		fmt.Fprintf(buf, "v%s = %s(%sptr%d)\n",
			genIndices("i", level), helper.Name, ptrs(goSpec.Pointers), level)
		return helper
	}
	var ref, ptr string
	if goSpec.Pointers == 0 {
		ptr = "*"
		ref = "&"
	}

	switch {
	case goSpec.Slices == 1 && goSpec.Pointers == 0 && cgoSpec.Pointers == 1:
		fmt.Fprintf(buf, "v%s = %snew%sRef(unsafe.Pointer(ptr%d))\n",
			genIndices("i", level), ptr, goSpec.Raw, level)
	default:
		fmt.Fprintf(buf, "v%s = %snew%sRef(unsafe.Pointer(%sptr%d))\n",
			genIndices("i", level), ptr, goSpec.Raw, ref, level)
	}
	// fmt.Fprintf(buf, "v%s = %snew%sRef(unsafe.Pointer(%sptr%d))\n",
	// 	genIndices("i", level), ptr, goSpec.Raw, ref, level)
	return nil
}

func packArray(buf1 io.Writer, buf2 *reverseBuffer, cgoSpec tl.CGoSpec, level uint8) {
	if level == 0 {
		fmt.Fprintln(buf1, "for i0 := range v {")
		fmt.Fprintf(buf1, "ptr1 := ptr0[i0]\n")
		buf2.Linef("}\n")
		return
	}
	fmt.Fprintf(buf1, "for i%d := range v%s {\n", level, genIndices("i", level))
	fmt.Fprintf(buf1, "ptr%d := ptr%d[i%d]\n", level+1, level, level)
	buf2.Linef("}\n")
}

func (gen *Generator) packSlice(buf1 io.Writer, buf2 *reverseBuffer, cgoSpec tl.CGoSpec, sizeConst string, level uint8) {
	cgoSpecLevel := cgoSpec.AtLevel(level + 1)
	if level == 0 {
		fmt.Fprintf(buf1, "const m = %s\n", gen.maxMem)
		fmt.Fprintln(buf1, "for i0 := range v {")
		fmt.Fprintf(buf1, "ptr1 := (*(*[m/%s]%s)(unsafe.Pointer(ptr0)))[i0]\n", sizeConst, cgoSpecLevel)
		buf2.Linef("}\n")
		return
	}
	fmt.Fprintf(buf1, "for i%d := range v%s {\n", level, genIndices("i", level))
	fmt.Fprintf(buf1, "ptr%d := (*(*[m/%s]%s)(unsafe.Pointer(ptr%d)))[i%d]\n",
		level+1, sizeConst, cgoSpecLevel, level, level)
	buf2.Linef("}\n")
}

func (gen *Generator) packSliceEx(buf1 io.Writer, buf2 *reverseBuffer, cgoSpec tl.CGoSpec, sizeConst string, level uint8) {
	cgoSpecLevel := cgoSpec.AtLevel(level + 1)
	if level == 0 {
		fmt.Fprintln(buf1, "// c struct pointer offset")
		fmt.Fprintln(buf1, "for i0 := range v {")
		fmt.Fprintf(buf1, "ptr1 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(i0)*uintptr(%s)))\n", cgoSpec, sizeConst)
		buf2.Linef("}\n")
		return
	}
	fmt.Fprintf(buf1, "for i%d := range v%s {\n", level, genIndices("i", level))
	fmt.Fprintf(buf1, "ptr%d := (*(*[m/%s]%s)(unsafe.Pointer(ptr%d)))[i%d]\n",
		level+1, sizeConst, cgoSpecLevel, level, level)
	buf2.Linef("}\n")
}

func (gen *Generator) getPackStringHelper(cgoSpec tl.CGoSpec) *Helper {
	cgoSpec = tl.CGoSpec{
		Pointers: 1,
		Base:     cgoSpec.Base,
	}
	name := "pack" + gen.getTypedHelperName("string", cgoSpec)
	return &Helper{
		Name:        name,
		Description: fmt.Sprintf("%s creates a Go string backed by %s and avoids copying.", name, cgoSpec),
		Source: fmt.Sprintf(`func %s(p %s) (raw string) {
			if p != nil && *p != 0 {
				h := (*stringHeader)(unsafe.Pointer(&raw))
				h.Data = unsafe.Pointer(p)
				for *p != 0 {
					p = (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + 1)) // p++
				}
				h.Len = int(uintptr(unsafe.Pointer(p)) - uintptr(h.Data))
			}
			return
		}`, name, cgoSpec, cgoSpec),
		Requires: Helpers{stringHeader, rawString},
	}
}

func (gen *Generator) getPackHelper(memTip tl.Tip, goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) *Helper {
	name := "pack" + getHelperName(goSpec)
	code := new(bytes.Buffer)
	var ref string
	if len(goSpec.OuterArr) > 0 {
		ref = "*"
	}
	fmt.Fprintf(code, "func %s(v %s%s, ptr0 %s%s) {\n", name, ref, goSpec, ref, cgoSpec)
	h := &Helper{
		Name:        name,
		Description: fmt.Sprintf("%s reads sliced Go data structure out from plain C format.", name),
	}
	var level uint8
	buf1 := new(bytes.Buffer)
	buf2 := new(reverseBuffer)

	for range goSpec.OuterArr.Sizes() {
		packArray(buf1, buf2, cgoSpec, level)
		level++
	}
	goSpec.OuterArr = ""

	gen.submitHelper(sizeOfPtr)
	getSizeSpec := func(level uint8) string {
		sizeConst := "sizeOfPtr"
		ptrs := cgoSpec.PointersAtLevel(level)
		if ptrs == 0 {
			// whoa, we're dealing with a value
			sizeConst = "sizeOf" + gen.getTypedHelperName("value", cgoSpec.SpecAtLevel(level))
		}
		return sizeConst
	}
	for goSpec.Slices > 1 {
		goSpec.Slices--
		gen.packSlice(buf1, buf2, cgoSpec, getSizeSpec(level+1), level)
		level++
	}
	isSlice := goSpec.Slices > 0
	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()

	switch {
	case isPlain && isSlice:
		gen.submitHelper(sliceHeader)
		gen.packPlainSlice(buf1, goSpec.PlainType(), goSpec.Pointers, level)
	case isPlain:
		packPlain(buf1, cgoSpec, goSpec.PlainType(), goSpec.Pointers, level)
	case isSlice:
		if goSpec.Slices == 1 && goSpec.Pointers == 0 && cgoSpec.Pointers == 1 {
			gen.packSliceEx(buf1, buf2, cgoSpec, getSizeSpec(level+1), level)
		} else {
			gen.packSlice(buf1, buf2, cgoSpec, getSizeSpec(level+1), level)
			goSpec.Slices = 0
		}
		// goSpec.Slices = 0
		if helper := gen.packObj(buf1, goSpec, cgoSpec, level+1); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	default:
		if helper := gen.packObj(buf1, goSpec, cgoSpec, level); helper != nil {
			h.Requires = append(h.Requires, helper)
		}
	}

	buf1.WriteTo(code)
	buf2.WriteTo(code)
	fmt.Fprintln(code, "}")
	h.Source = code.String()
	return h
}

func (gen *Generator) proxyArgToGo(memTip tl.Tip, varName, ptrName string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {
	nillable = true

	if goSpec.IsGoString() {
		helper := gen.getPackStringHelper(cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s = %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	} else if getHelper, ok := toGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s = %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	}

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
		isPlain && goSpec.Slices > 1:                             // ex: [][]byte
		helper := gen.getPackHelper(memTip, goSpec, cgoSpec)
		gen.submitHelper(helper)
		if len(goSpec.OuterArr) > 0 {
			ptrName = fmt.Sprintf("(*%s)(unsafe.Pointer(%s))", cgoSpec, ptrName)
		}
		proxy = fmt.Sprintf("%s(%s, %s)", helper.Name, varName, ptrName)
		return proxy, helper.Nillable
	case isPlain && goSpec.Slices != 0: // ex: []byte
		// skip because slice data can be edited either way
		return
	case isPlain: // ex: byte, [4]byte
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("*%s = *(%s)(%s)", varName, goSpec, ptrName)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			proxy = fmt.Sprintf("// %s is a callback func", varName)
			return
		}
		proxy = fmt.Sprintf("*%s = *(*%s)(unsafe.Pointer(&%s))", varName, goSpec, ptrName)
		return
	default: // ex: *SomeType
		proxy = fmt.Sprintf("*%s = *(new%sRef(unsafe.Pointer(%s)))", varName, goSpec.Raw, ptrName)
		return
	}
}

func (gen *Generator) proxyValueToGo(memTip tl.Tip, varName, ptrName string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {
	nillable = true

	if goSpec.IsGoString() {
		helper := gen.getPackStringHelper(cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s = %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	} else if getHelper, ok := toGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s = %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	}
	gen.submitHelper(cgoAllocMap)

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
		isPlain && goSpec.Slices > 1:                             // ex: [][]byte
		helper := gen.getPackHelper(memTip, goSpec, cgoSpec)
		gen.submitHelper(sliceHeader)
		gen.submitHelper(helper)
		if len(goSpec.OuterArr) > 0 {
			ptrName = fmt.Sprintf("(*%s)(unsafe.Pointer(&%s))", cgoSpec, ptrName)
		}
		var ref string
		switch {
		case len(goSpec.OuterArr) > 0,
			goSpec.Pointers == 0 && goSpec.Slices == 0:
			ref = "&"
		}
		proxy = fmt.Sprintf("%s(%s%s, %s)", helper.Name, ref, varName, ptrName)
		return proxy, helper.Nillable
	case isPlain && goSpec.Slices == 1: // ex: []byte
		buf := new(bytes.Buffer)
		gen.submitHelper(sliceHeader)

		goSpecS0P1 := goSpec
		goSpecS0P1.Slices -= 1

		goSpecS0P0 := goSpec
		goSpecS0P0.Slices -= 1
		goSpecS0P0.Pointers -= 1

		cgoSpecP0 := cgoSpec
		cgoSpecP0.Pointers -= 2
		cgoSpecP1 := cgoSpec
		cgoSpecP1.Pointers -= 1
		fmt.Fprintf(buf, `
			const sizeOfPlainValue = unsafe.Sizeof([1]%s{})
			ptr0 := x.%s
			ptr1 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(ptr0)) + uintptr(index)*uintptr(sizeOfPlainValue)))
			ret = (%s)(unsafe.Pointer(ptr1))
			`, cgoSpecP0, ptrName, cgoSpecP1, goSpecS0P1)

		proxy = buf.String()
		return
	case isPlain && goSpec.Slices > 1: // ex: [][4]byte
		fmt.Printf("[WARN] goSpec: %s plain: %s slices: %d\n", varName, goSpec.PlainType(), goSpec.Slices)
		gen.submitHelper(sliceHeader)
		buf := new(bytes.Buffer)
		postfix := gen.randPostfix()
		fmt.Fprintf(buf, "hx%2x := (*sliceHeader)(unsafe.Pointer(&%s))\n", postfix, varName)
		fmt.Fprintf(buf, "hx%2x.Data = unsafe.Pointer(%s)\n", postfix, ptrName)
		fmt.Fprintf(buf, "hx%2x.Cap = %s\n", postfix, gen.maxMem)
		fmt.Fprintf(buf, "// hx%2x.Len = ?\n", postfix)
		proxy = buf.String()
		return
	case isPlain: // ex: byte, [4]byte
		var ref, ptr string
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("%s = (%s)(%s)", varName, goSpec, ptrName)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			proxy = fmt.Sprintf("// %s is a callback func", varName)
			return
		} else if goSpec.Pointers == 0 || len(goSpec.OuterArr) > 0 {
			if ptrName[0] != '&' {
				ref = "&"
			}
			ptr = "*"
		}
		proxy = fmt.Sprintf("%s = %s(%s%s)(unsafe.Pointer(%s%s))", varName, ptr, ptr, goSpec, ref, ptrName)
		return
	default: // ex: *SomeType
		var ref, deref string
		if cgoSpec.Pointers == 0 {
			deref = "*"
			ref = "&"
		}
		proxy = fmt.Sprintf("%s = %snew%sRef(unsafe.Pointer(%s%s))", varName, deref, goSpec.Raw, ref, ptrName)
		return
	}
}

func (gen *Generator) proxyRetToGo(wr io.Writer, decl *tl.CDecl, memTip tl.Tip, varName, ptrName string,
	goSpec tl.GoTypeSpec, cgoSpec tl.CGoSpec) (proxy string, nillable bool) {

	if goSpec.IsGoString() {
		helper := gen.getPackStringHelper(cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s := %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	} else if getHelper, ok := toGoHelperMap[goSpec]; ok {
		helper := getHelper(gen, cgoSpec)
		gen.submitHelper(helper)
		proxy = fmt.Sprintf("%s := %s(%s)", varName, helper.Name, ptrName)
		return proxy, helper.Nillable
	}
	gen.submitHelper(cgoAllocMap)

	isPlain := (memTip == tl.TipMemRaw) || goSpec.IsPlain() || goSpec.IsPlainKind()
	switch {
	case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
		isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0: // ex: [4][]byte
		helper := gen.getPackHelper(memTip, goSpec, cgoSpec)
		gen.submitHelper(helper)
		var ref string
		if len(goSpec.OuterArr) > 0 {
			ref = "&"
		}

		if goSpec.Slices == 1 && goSpec.Kind == tl.StructKind {
			fmt.Fprintf(wr, "var %s %s\n", varName, goSpec)
			fmt.Fprintf(wr, "// completion of the function\n")
			fmt.Fprintf(wr, "%s = make(%s, %sLength", varName, goSpec, unexportName(decl.Name))
			writeStartParams(wr)
			gen.writeFunctionParamsEx(wr, "", decl.Spec)
			writeEndParams(wr)
			fmt.Fprintf(wr, ")\n")
			proxy = fmt.Sprintf("%s(%s%s, %s)", helper.Name, ref, varName, ptrName)
		} else {
			proxy = fmt.Sprintf("var %s %s\n%s(%s%s, %s)", varName, goSpec, helper.Name, ref, varName, ptrName)
		}
		// proxy = fmt.Sprintf("var %s %s\n%s(%s%s, %s)", varName, goSpec, helper.Name, ref, varName, ptrName)
		return proxy, helper.Nillable
	case isPlain && goSpec.Slices > 1: // ex: [][]byte
		goSpecS1 := goSpec
		goSpecS1.Slices -= 1

		goSpecS0P1 := goSpec
		goSpecS0P1.Slices -= 2
		goSpecS0P1.Pointers += 1

		cgoSpecP1 := cgoSpec
		cgoSpecP1.Pointers -= 1

		// helper := gen.getPackHelper(memTip, goSpec, cgoSpec)
		// gen.submitHelper(helper)
		var ref string
		if len(goSpec.OuterArr) > 0 {
			ref = "&"
		}
		if goSpec.Base == "byte" {
			buf := new(bytes.Buffer)
			fmt.Fprint(buf, "ptrRow, ")
			gen.writeFunctionParamsEx(buf, "", decl.Spec)

			proxy = fmt.Sprintf(`
				const sizeOfPlainValue = unsafe.Sizeof([1]%s{})
				__v := make(%s, *count)
				
				for i0 := range __v {
					ptr1 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(__ret)) + uintptr(i0)*uintptr(sizeOfPtr)))
					ptrRow := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(*ptr1))))
					// completion of the function
					__v[i0] = make([]byte, %sLength(%s))
					for i1 := range __v[i0] {
						ptr2 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(*ptr1)) + uintptr(i1)*uintptr(sizeOfPlainValue)))
						__v[i0][i1] = (%s)(unsafe.Pointer(ptr2))
					}
				}`, cgoSpec.Base, goSpec, cgoSpec, cgoSpecP1, unexportName(decl.Name), buf.String(), cgoSpecP1, goSpecS0P1)
		} else {
			helper := gen.getPackHelper(memTip, goSpec, cgoSpec)
			gen.submitHelper(helper)
			proxy = fmt.Sprintf("var %s %s\n%s(%s%s, %s)", varName, goSpec, helper.Name, ref, varName, ptrName)
		}
		// proxy = fmt.Sprintf("var %s %s\n%s(%s%s, %s)", varName, goSpec, helper.Name, ref, varName, ptrName)
		// return proxy, helper.Nillable
		return
	// case isPlain && goSpec.Slices != 0: // ex: []byte
	// 	specStr := ptrs(goSpec.Pointers) + goSpec.PlainType()
	// 	proxy = fmt.Sprintf("%s := (*(*[%s]%s)(unsafe.Pointer(%s)))[:0]",
	// 		varName, gen.maxMem, specStr, ptrName)
	// 	return
	case isPlain && goSpec.Slices != 0: // ex: []byte
		goSpecS0P1 := goSpec
		goSpecS0P1.Slices -= 1
		goSpecS0P1.Pointers += 1

		if goSpec.Base == "byte" {
			buf := new(bytes.Buffer)
			gen.writeFunctionParamsEx(buf, "", decl.Spec)

			proxy = fmt.Sprintf(`
				const sizeOfPlainValue = unsafe.Sizeof([1]%s{})
				// completion of the function
				__v := make([]byte, %sLength(%s))
				for i0 := range __v {
					ptr0 := (%s)(unsafe.Pointer(uintptr(unsafe.Pointer(__ret)) + uintptr(i0)*uintptr(sizeOfPlainValue)))
					__v[i0] = (%s)(unsafe.Pointer(ptr0))
				}`, cgoSpec.Base, unexportName(decl.Name), buf.String(), cgoSpec, goSpecS0P1)
		} else {
			specStr := ptrs(goSpec.Pointers) + goSpec.PlainType()
			proxy = fmt.Sprintf("%s := (*(*[%s]%s)(unsafe.Pointer(%s)))[:0]",
				varName, gen.maxMem, specStr, ptrName)
		}

		return
	case isPlain: // ex: byte, [4]byte
		if (goSpec.Kind == tl.PlainTypeKind || goSpec.Kind == tl.EnumKind) &&
			len(goSpec.OuterArr)+len(goSpec.InnerArr) == 0 && goSpec.Pointers == 0 {
			proxy = fmt.Sprintf("%s := (%s)(%s)", varName, goSpec, ptrName)
			return
		} else if goSpec.Kind == tl.FunctionKind {
			proxy = fmt.Sprintf("// %s is a callback func", varName)
			return
		}
		proxy = fmt.Sprintf("%s := *(*%s)(unsafe.Pointer(&%s))", varName, goSpec, ptrName)
		return
	default: // ex: *SomeType
		var deref, ref string
		if cgoSpec.Pointers == 0 {
			deref = "*"
			ref = "&"
		}
		proxy = fmt.Sprintf("%s := %snew%sRef(unsafe.Pointer(%s%s)).convert()", varName, deref, goSpec.Raw, ref, ptrName)
		// proxy = fmt.Sprintf("%s := %snew%sRef(unsafe.Pointer(%s%s))", varName, deref, goSpec.Raw, ref, ptrName)
		return
	}
}

func (gen *Generator) createProxies(funcName string, funcSpec tl.CType) (from, to []proxyDecl) {
	spec := funcSpec.(*tl.CFunctionSpec)
	from = make([]proxyDecl, len(spec.Params))
	to = make([]proxyDecl, 0, len(spec.Params))

	cNamesSeen := make(map[string]struct{}, len(spec.Params))
	for _, param := range spec.Params {
		if len(param.Name) > 0 {
			cNamesSeen[param.Name] = struct{}{}
		}
	}

	ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeFunction, funcName, funcSpec)
	for i, param := range spec.Params {
		var goSpec tl.GoTypeSpec
		ptrTip := ptrTipRx.TipAt(i)
		typeTip := typeTipRx.TipAt(i)
		goSpec = gen.tr.TranslateSpec(param.Spec, ptrTip, typeTip)
		cgoSpec := gen.tr.CGoSpec(param.Spec, true)
		const public = false
		refName := string(gen.tr.TransformName(tl.TargetType, param.Name, public))
		fromBuf := new(bytes.Buffer)
		toBuf := new(bytes.Buffer)

		name := "c" + refName
		_, seen := cNamesSeen[name]
		for seen {
			name = "c" + name
			_, seen = cNamesSeen[name]
		}
		cNamesSeen[name] = struct{}{}

		argTip := memTipRx.TipAt(i)
		if !argTip.IsValid() {
			argTip = gen.MemTipOf(param)
		}
		var needKeepalive bool
		if gen.cfg.Options.SafeStrings && goSpec.IsGoString() {
			needKeepalive = true
			gen.submitHelper(safeString)
			fmt.Fprintf(fromBuf, "%s = safeString(%s)\n", refName, refName)
		}
		fromProxy, nillable := gen.proxyArgFromGo(argTip, refName, goSpec, cgoSpec)
		if nillable {
			fmt.Fprintf(fromBuf, "var %s %s\n", name, cgoSpec)
			fmt.Fprintf(fromBuf, "if %s != nil {\n%s, _ = %s\n}", refName, name, fromProxy)
		} else {
			fmt.Fprintf(fromBuf, "%s, _ := %s", name, fromProxy)
		}
		from[i] = proxyDecl{Name: name, Decl: fromBuf.String()}
		if needKeepalive {
			keepaliveDecl := fmt.Sprintf("runtime.KeepAlive(%s)\n", refName)
			to = append(to, proxyDecl{Name: refName, Decl: keepaliveDecl})
		}

		isPlain := goSpec.IsPlain() || goSpec.IsPlainKind()
		switch {
		case !isPlain && (goSpec.Slices > 0 || len(goSpec.OuterArr) > 0), // ex: []string
			isPlain && goSpec.Slices > 0 && len(goSpec.OuterArr) > 0, // ex: [4][]byte
			isPlain && goSpec.Slices > 1:                             // ex: [][]byte
			// need to re-pack values into Go memory layout
			toProxy, nillable := gen.proxyArgToGo(argTip, refName, name, goSpec, cgoSpec)
			if len(toProxy) > 0 {
				if nillable {
					fmt.Fprintf(toBuf, "if %s != nil {\n%s\n}", refName, toProxy)
				} else {
					fmt.Fprintln(toBuf, toProxy)
				}
				to = append(to, proxyDecl{Name: name, Decl: toBuf.String()})
			}
		}
	}
	return
}

func (gen *Generator) submitHelper(h *Helper) {
	if h == nil {
		return
	}
	gen.helpersChan <- h
	reqs := h.Requires
	for len(reqs) > 0 {
		var newReqs Helpers
		for _, req := range reqs {
			gen.helpersChan <- req
			newReqs = append(newReqs, req.Requires...)
		}
		reqs = newReqs
	}
}

func (gen *Generator) writeFunctionBody(wr io.Writer, decl *tl.CDecl) {
	writeStartFuncBody(wr)
	wr2 := new(reverseBuffer)
	from, to := gen.createProxies(decl.Name, decl.Spec)
	for _, proxy := range from {
		fmt.Fprintln(wr, proxy.Decl)
	}
	for _, proxy := range to {
		// wr2 will be handled below
		wr2.Line(proxy.Decl)
	}
	spec := decl.Spec.(*tl.CFunctionSpec)
	if spec.Return != nil {
		fmt.Fprint(wr, "__ret := ")
	}
	fmt.Fprintf(wr, "C.%s", decl.Name)
	writeStartParams(wr)
	for i := range spec.Params {
		fmt.Fprint(wr, from[i].Name)
		if i < len(spec.Params)-1 {
			fmt.Fprint(wr, ", ")
		}
	}
	writeEndParams(wr)
	writeSpace(wr, 1)
	// wr2 being populated above
	wr2.WriteTo(wr)
	if spec.Return != nil {
		ptrTipRx, typeTipRx, memTipRx := gen.tr.TipRxsForSpec(tl.TipScopeFunction, decl.Name, decl.Spec)
		ptrTip := ptrTipRx.Self()
		typeTip := typeTipRx.Self()
		if !ptrTip.IsValid() {
			// defaults to ref for the returns
			ptrTip = tl.TipPtrRef
		}
		goSpec := gen.tr.TranslateSpec((*spec).Return, ptrTip, typeTip)
		cgoSpec := gen.tr.CGoSpec((*spec).Return, false)

		if goSpec.Base == "string" && goSpec.Pointers == 1 {
			retProxy, nillable := gen.proxyRetToGo(wr, decl, memTipRx.Self(), "__v", "*__ret", goSpec, cgoSpec)
			if nillable {
				fmt.Fprintln(wr, "if ret == nil {\nreturn nil\n}")
			}

			fmt.Fprintln(wr, retProxy)
			fmt.Fprintln(wr, "return &__v")
		} else {
			retProxy, nillable := gen.proxyRetToGo(wr, decl, memTipRx.Self(), "__v", "__ret", goSpec, cgoSpec)
			if nillable {
				fmt.Fprintln(wr, "if ret == nil {\nreturn nil\n}")
			}

			fmt.Fprintln(wr, retProxy)
			fmt.Fprintln(wr, "return __v")
		}
	}
	writeEndFuncBody(wr)
}

var (
	cgoGenTag = &Helper{
		Name:   "cgoGenTag",
		Source: "#define __CGOGEN 1",
		Side:   CHSide,
	}
	sizeOfPtr = &Helper{
		Name:   "sizeOfPtr",
		Source: "const sizeOfPtr = unsafe.Sizeof(&struct{}{})",
	}
	sliceHeader = &Helper{
		Name: "sliceHeader",
		Source: `type sliceHeader struct {
			Data unsafe.Pointer
			Len  int
			Cap  int
		}`,
	}
	stringHeader = &Helper{
		Name: "stringHeader",
		Source: `type stringHeader struct {
			Data unsafe.Pointer
			Len  int
		}`,
	}
	rawString = &Helper{
		Name:        "RawString",
		Description: "RawString reperesents a string backed by data on the C side.",
		Source:      `type RawString string`,
		Requires:    Helpers{rawStringCopy},
	}
	rawStringCopy = &Helper{
		Name:        "RawString.Copy",
		Description: "Copy returns a Go-managed copy of raw string.",
		Source: `func (raw RawString) Copy() string {
			if len(raw) == 0 {
				return ""
			}
			h := (*stringHeader)(unsafe.Pointer(&raw))
			return C.GoStringN((*C.char)(h.Data), C.int(h.Len))
		}`,
		Requires: []*Helper{stringHeader},
	}
	safeString = &Helper{
		Name:        "safeString",
		Description: `safeString ensures that the string is NULL-terminated, a NULL-terminated copy is created otherwise.`,
		Source: `func safeString(str string) string {
			if len(str) > 0 && str[len(str)-1] != '\x00' {
				str = str + "\x00"
			} else if len(str) == 0 {
				str = "\x00"
			}
			return str
		}`,
	}
	cgoAllocMap = &Helper{
		Name:        "cgoAllocMap",
		Description: "cgoAllocMap stores pointers to C allocated memory for future reference.",
		Source: `type cgoAllocMap struct {
			mux sync.RWMutex
			m   map[unsafe.Pointer]struct{}
		}

		var cgoAllocsUnknown = new(cgoAllocMap)
		var allocReferenceCount int

		func init() {
			allocReferenceCount = 0
		}

		func (a *cgoAllocMap) Add(ptr unsafe.Pointer) {
			a.mux.Lock()
			defer a.mux.Unlock()

			if a.m == nil {
				a.m = make(map[unsafe.Pointer]struct{})
			}
			a.m[ptr] = struct{}{}

			allocReferenceCount++
			fmt.Printf("INFO: MEMORY: [PTR %p] CGO memory alloc\n", ptr)
		}

		func (a *cgoAllocMap) IsEmpty() bool {
			a.mux.RLock()
			defer a.mux.RUnlock()

			isEmpty := len(a.m) == 0
			return isEmpty
		}

		func (a *cgoAllocMap) Borrow(b *cgoAllocMap) {
			if b == nil || b.IsEmpty() {
				return
			}

			b.mux.Lock()
			defer b.mux.Unlock()
			a.mux.Lock()
			defer a.mux.Unlock()

			for ptr := range b.m {
				if a.m == nil {
					a.m = make(map[unsafe.Pointer]struct{})
				}
				a.m[ptr] = struct{}{}
				delete(b.m, ptr)
			}
		}

		func (a *cgoAllocMap) Free() {
			a.mux.Lock()
			defer a.mux.Unlock()

			for ptr := range a.m {
				C.free(ptr)
				delete(a.m, ptr)

				allocReferenceCount--
				fmt.Printf("INFO: MEMORY: [PTR %p] CGO memory free\n", ptr)
			}
		}`,
	}
)

package parser

import (
	"strings"

	"modernc.org/cc"
)

type TargetArch string

const (
	Arch32    TargetArch = "i386"
	Arch48    TargetArch = "x86_48"
	Arch64    TargetArch = "x86_64"
	ArchArm32 TargetArch = "arm"
	ArchArm64 TargetArch = "aarch64"
)

var builtinBase = `
#define __builtin_va_list void *
#define __asm(x)
#define __inline
#define __inline__
#define __signed
#define __signed__
#define __const const
#define __extension__
#define __attribute__(x)
#define __attribute(x)
#define __restrict
#define __volatile__

#define __builtin_inff() (0)
#define __builtin_infl() (0)
#define __builtin_inf() (0)
#define __builtin_fabsf(x) (0)
#define __builtin_fabsl(x) (0)
#define __builtin_fabs(x) (0)

#define __INTRINSIC_PROLOG(name)
`

var builtinBaseUndef = `
#undef __llvm__
#undef __BLOCKS__
`

var basePredefines = `
#define __STDC_HOSTED__ 1
#define __STDC_VERSION__ 199901L
#define __STDC__ 1
#define __GNUC__ 4
#define __GNUC_PREREQ(maj,min) 0
#define __POSIX_C_DEPRECATED(ver)
#define __has_include_next(...) 1

#define __FLT_MIN__ 0
#define __DBL_MIN__ 0
#define __LDBL_MIN__ 0

void __GO__(char*, ...);
`

var archPredefines = map[TargetArch]string{
	Arch32: `#define __i386__ 1`,
	Arch48: `#define __x86_64__ 1`,
	Arch64: `#define __x86_64__ 1`,
	ArchArm32: strings.Join([]string{
		`#define __ARM_EABI__ 1`,
		`#define __arm__ 1`,
	}, "\n"),
	ArchArm64: strings.Join([]string{
		`#define __ARM_EABI__ 1`,
		`#define __aarch64__ 1`,
	}, "\n"),
	// TODO(xlab): https://sourceforge.net/p/predef/wiki/Architectures/
}

var models = map[TargetArch]*cc.Model{
	Arch32:    model32,
	Arch48:    model48,
	Arch64:    model64,
	ArchArm32: model32,
	ArchArm64: model64,
}

var arches = map[string]TargetArch{
	"386":         Arch32,
	"arm":         ArchArm32,
	"aarch64":     ArchArm64,
	"armv7a":      ArchArm32,
	"armv8a":      ArchArm64,
	"armeabi-v7a": ArchArm32,
	"armeabi-v8a": ArchArm64,
	"armbe":       ArchArm32,
	"mips":        Arch32,
	"mipsle":      Arch32,
	"sparc":       Arch32,
	"amd64":       Arch64,
	"amd64p32":    ArchArm32,
	"arm64":       ArchArm64,
	"arm64be":     ArchArm64,
	"ppc64":       Arch64,
	"ppc64le":     Arch64,
	"mips64":      Arch64,
	"mips64le":    Arch64,
	"mips64p32":   Arch48,
	"mips64p32le": Arch48,
	"sparc64":     Arch64,
}

var model32 = &cc.Model{
	Items: map[cc.Kind]cc.ModelItem{
		cc.Ptr:               {Size: 4, Align: 4, StructAlign: 4, More: "__TODO_PTR"},
		cc.UintPtr:           {Size: 4, Align: 4, StructAlign: 4, More: "uintptr"},
		cc.Void:              {Size: 0, Align: 1, StructAlign: 1, More: "__TODO_VOID"},
		cc.Char:              {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.SChar:             {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.UChar:             {Size: 1, Align: 1, StructAlign: 1, More: "byte"},
		cc.Short:             {Size: 2, Align: 2, StructAlign: 2, More: "int16"},
		cc.UShort:            {Size: 2, Align: 2, StructAlign: 2, More: "uint16"},
		cc.Int:               {Size: 4, Align: 4, StructAlign: 4, More: "int32"},
		cc.UInt:              {Size: 4, Align: 4, StructAlign: 4, More: "uint32"},
		cc.Long:              {Size: 4, Align: 4, StructAlign: 4, More: "int32"},
		cc.ULong:             {Size: 4, Align: 4, StructAlign: 4, More: "uint32"},
		cc.LongLong:          {Size: 8, Align: 8, StructAlign: 8, More: "int64"},
		cc.ULongLong:         {Size: 8, Align: 8, StructAlign: 8, More: "uint64"},
		cc.Float:             {Size: 4, Align: 4, StructAlign: 4, More: "float32"},
		cc.Double:            {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.LongDouble:        {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.Bool:              {Size: 1, Align: 1, StructAlign: 1, More: "bool"},
		cc.FloatComplex:      {Size: 8, Align: 8, StructAlign: 8, More: "complex64"},
		cc.DoubleComplex:     {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
		cc.LongDoubleComplex: {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
	},
}

var model48 = &cc.Model{
	Items: map[cc.Kind]cc.ModelItem{
		cc.Ptr:               {Size: 4, Align: 4, StructAlign: 4, More: "__TODO_PTR"},
		cc.UintPtr:           {Size: 4, Align: 4, StructAlign: 4, More: "uintptr"},
		cc.Void:              {Size: 0, Align: 1, StructAlign: 1, More: "__TODO_VOID"},
		cc.Char:              {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.SChar:             {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.UChar:             {Size: 1, Align: 1, StructAlign: 1, More: "byte"},
		cc.Short:             {Size: 2, Align: 2, StructAlign: 2, More: "int16"},
		cc.UShort:            {Size: 2, Align: 2, StructAlign: 2, More: "uint16"},
		cc.Int:               {Size: 4, Align: 4, StructAlign: 4, More: "int32"},
		cc.UInt:              {Size: 4, Align: 4, StructAlign: 4, More: "uint32"},
		cc.Long:              {Size: 8, Align: 8, StructAlign: 8, More: "int64"},
		cc.ULong:             {Size: 8, Align: 8, StructAlign: 8, More: "uint64"},
		cc.LongLong:          {Size: 8, Align: 8, StructAlign: 8, More: "int64"},
		cc.ULongLong:         {Size: 8, Align: 8, StructAlign: 8, More: "uint64"},
		cc.Float:             {Size: 4, Align: 4, StructAlign: 4, More: "float32"},
		cc.Double:            {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.LongDouble:        {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.Bool:              {Size: 1, Align: 1, StructAlign: 1, More: "bool"},
		cc.FloatComplex:      {Size: 8, Align: 8, StructAlign: 8, More: "complex64"},
		cc.DoubleComplex:     {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
		cc.LongDoubleComplex: {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
	},
}

var model64 = &cc.Model{
	Items: map[cc.Kind]cc.ModelItem{
		cc.Ptr:               {Size: 8, Align: 8, StructAlign: 8, More: "__TODO_PTR"},
		cc.UintPtr:           {Size: 8, Align: 8, StructAlign: 8, More: "uintptr"},
		cc.Void:              {Size: 0, Align: 1, StructAlign: 1, More: "__TODO_VOID"},
		cc.Char:              {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.SChar:             {Size: 1, Align: 1, StructAlign: 1, More: "int8"},
		cc.UChar:             {Size: 1, Align: 1, StructAlign: 1, More: "byte"},
		cc.Short:             {Size: 2, Align: 2, StructAlign: 2, More: "int16"},
		cc.UShort:            {Size: 2, Align: 2, StructAlign: 2, More: "uint16"},
		cc.Int:               {Size: 4, Align: 4, StructAlign: 4, More: "int32"},
		cc.UInt:              {Size: 4, Align: 4, StructAlign: 4, More: "uint32"},
		cc.Long:              {Size: 8, Align: 8, StructAlign: 8, More: "int64"},
		cc.ULong:             {Size: 8, Align: 8, StructAlign: 8, More: "uint64"},
		cc.LongLong:          {Size: 8, Align: 8, StructAlign: 8, More: "int64"},
		cc.ULongLong:         {Size: 8, Align: 8, StructAlign: 8, More: "uint64"},
		cc.Float:             {Size: 4, Align: 4, StructAlign: 4, More: "float32"},
		cc.Double:            {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.LongDouble:        {Size: 8, Align: 8, StructAlign: 4, More: "float64"},
		cc.Bool:              {Size: 1, Align: 1, StructAlign: 1, More: "bool"},
		cc.FloatComplex:      {Size: 8, Align: 8, StructAlign: 8, More: "complex64"},
		cc.DoubleComplex:     {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
		cc.LongDoubleComplex: {Size: 16, Align: 16, StructAlign: 16, More: "complex128"},
	},
}

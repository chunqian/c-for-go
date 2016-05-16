package parser

import (
	"strings"

	"github.com/cznic/cc"
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
#define __asm__(x)
#define __asm(x)
#define __inline inline
#define __inline__ inline
#define __signed signed
#define __signed__ signed
#define __extension__
#define __attribute__(x)
#define __restrict
`

var basePredefines = `
#define __STDC_HOSTED__ 1
#define __STDC_VERSION__ 199901L
#define __STDC__ 1
#define __GNUC__ 4
#define __POSIX_C_DEPRECATED(ver)

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
		cc.Ptr:               {4, 4, 4, "__TODO_PTR"},
		cc.UintPtr:           {4, 4, 4, "__TODO_UINTPTR"},
		cc.Void:              {0, 1, 1, "__TODO_VOID"},
		cc.Char:              {1, 1, 1, "int8"},
		cc.SChar:             {1, 1, 1, "int8"},
		cc.UChar:             {1, 1, 1, "byte"},
		cc.Short:             {2, 2, 2, "int16"},
		cc.UShort:            {2, 2, 2, "uint16"},
		cc.Int:               {4, 4, 4, "int32"},
		cc.UInt:              {4, 4, 4, "uint32"},
		cc.Long:              {4, 4, 4, "int32"},
		cc.ULong:             {4, 4, 4, "uint32"},
		cc.LongLong:          {8, 8, 8, "int64"},
		cc.ULongLong:         {8, 8, 8, "uint64"},
		cc.Float:             {4, 4, 4, "float32"},
		cc.Double:            {8, 8, 8, "float64"},
		cc.LongDouble:        {8, 8, 8, "float64"},
		cc.Bool:              {1, 1, 1, "bool"},
		cc.FloatComplex:      {8, 8, 8, "complex64"},
		cc.DoubleComplex:     {16, 16, 16, "complex128"},
		cc.LongDoubleComplex: {16, 16, 16, "complex128"},
	},
}

var model48 = &cc.Model{
	Items: map[cc.Kind]cc.ModelItem{
		cc.Ptr:               {4, 4, 4, "__TODO_PTR"},
		cc.UintPtr:           {4, 4, 4, "__TODO_UINTPTR"},
		cc.Void:              {0, 1, 1, "__TODO_VOID"},
		cc.Char:              {1, 1, 1, "int8"},
		cc.SChar:             {1, 1, 1, "int8"},
		cc.UChar:             {1, 1, 1, "byte"},
		cc.Short:             {2, 2, 2, "int16"},
		cc.UShort:            {2, 2, 2, "uint16"},
		cc.Int:               {4, 4, 4, "int32"},
		cc.UInt:              {4, 4, 4, "uint32"},
		cc.Long:              {8, 8, 8, "int64"},
		cc.ULong:             {8, 8, 8, "uint64"},
		cc.LongLong:          {8, 8, 8, "int64"},
		cc.ULongLong:         {8, 8, 8, "uint64"},
		cc.Float:             {4, 4, 4, "float32"},
		cc.Double:            {8, 8, 8, "float64"},
		cc.LongDouble:        {8, 8, 8, "float64"},
		cc.Bool:              {1, 1, 1, "bool"},
		cc.FloatComplex:      {8, 8, 8, "complex64"},
		cc.DoubleComplex:     {16, 16, 16, "complex128"},
		cc.LongDoubleComplex: {16, 16, 16, "complex128"},
	},
}

var model64 = &cc.Model{
	Items: map[cc.Kind]cc.ModelItem{
		cc.Ptr:               {8, 8, 8, "__TODO_PTR"},
		cc.UintPtr:           {8, 8, 8, "__TODO_UINTPTR"},
		cc.Void:              {0, 1, 1, "__TODO_VOID"},
		cc.Char:              {1, 1, 1, "int8"},
		cc.SChar:             {1, 1, 1, "int8"},
		cc.UChar:             {1, 1, 1, "byte"},
		cc.Short:             {2, 2, 2, "int16"},
		cc.UShort:            {2, 2, 2, "uint16"},
		cc.Int:               {4, 4, 4, "int32"},
		cc.UInt:              {4, 4, 4, "uint32"},
		cc.Long:              {8, 8, 8, "int64"},
		cc.ULong:             {8, 8, 8, "uint64"},
		cc.LongLong:          {8, 8, 8, "int64"},
		cc.ULongLong:         {8, 8, 8, "uint64"},
		cc.Float:             {4, 4, 4, "float32"},
		cc.Double:            {8, 8, 8, "float64"},
		cc.LongDouble:        {8, 8, 8, "float64"},
		cc.Bool:              {1, 1, 1, "bool"},
		cc.FloatComplex:      {8, 8, 8, "complex64"},
		cc.DoubleComplex:     {16, 16, 16, "complex128"},
		cc.LongDoubleComplex: {16, 16, 16, "complex128"},
	},
}

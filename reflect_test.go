package geerpc

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type Test struct {
	Name string
	Age  int
}

func (t *Test) GrowUp() int {
	t.Age++
	return t.Age
}

func (t *Test) SetName(newName string) {
	t.Name = newName
}

func TestName(t *testing.T) {
	test := Test{
		Name: "hhz",
		Age:  23,
	}
	//a1 := "aaa"
	//a2 := "bbb"
	//typeT := reflect.TypeOf(&test)
	valueT := reflect.ValueOf(&test)
	fmt.Println("=====")
	//fmt.Println(typeT.Elem())
	//fmt.Println(valueT.Elem())
	//t1 := reflect.TypeOf(a1)
	//t2 := reflect.TypeOf(&a2)
	//fmt.Println(reflect.New(t1).Elem())
	//fmt.Println(reflect.New(t2.Elem()))
	fmt.Println(reflect.Indirect(valueT))
	fmt.Println(reflect.Indirect(valueT).Type())
	fmt.Println(reflect.Indirect(valueT).Type().Name())
	fmt.Println("=====")
}

func TestDot(t *testing.T) {
	serviceMethod := "Test.GrowUp"
	index := strings.LastIndex(serviceMethod, ".")
	fmt.Println(index)
}

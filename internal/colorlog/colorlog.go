package colorlog

import (
	"fmt"
	"log"
)

// Multi - логирование нескольких аргументов
func Multi(title string, args ...interface{}) {
	str := fmt.Sprintf("\033[36m%v:\033[0m\n", title)
	for i, v := range args {
		str += fmt.Sprintf("\033[35m%v)\033[0m \033[34m%T\033[0m %#v\n", i+1, v, v)
	}
	log.Print(str)
}

// Solo - логирование одного аргумента
func Solo(title string, arg interface{}) {
	log.Printf("\033[36m%s:\033[0m %+v\n", title, arg)
}

// SoloT - логирование одного аргумента с выводом типа
func SoloT(title string, arg interface{}) {
	log.Printf("\033[36m%s:\033[0m \033[34m%T\033[0m %+v\n", title, arg, arg)
}

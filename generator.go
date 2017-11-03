package gowsdl

import (
	"bytes"
	"go/format"
	"log"
	"os"
	"path"
)

type Generator struct {
	WsdlPath             string
	Pkg                  string
	InsecureTLS          bool
	MakePublic           bool
	Login                string
	Password             string
	IgnoreTypeNamespaces bool
	OutFile              string
}

func (r *Generator) Generate() (err error) {
	// load wsdl
	goWsdl, err := NewGoWSDL(r.WsdlPath, r.Pkg, r.InsecureTLS, r.MakePublic)
	if err != nil {
		log.Println(err)
		return
	}
	if len(r.Login) > 0 && len(r.Password) > 0 {
		goWsdl.SetBasicAuth(r.Login, r.Password)
	}
	goWsdl.SetIgnoreTypeNamespaces(r.IgnoreTypeNamespaces)

	// generate code
	goCode, err := goWsdl.Start()
	if err != nil {
		log.Println(err)
		return
	}

	pkgDir := path.Join(".", r.Pkg)
	err = os.Mkdir(pkgDir, 0744)

	file, err := os.Create(path.Join(pkgDir, r.OutFile))
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	data := new(bytes.Buffer)
	data.Write(goCode["header"])
	data.Write(goCode["types"])
	data.Write(goCode["operations"])
	data.Write(goCode["soap"])

	// go fmt the generated code
	source, err := format.Source(data.Bytes())
	if err != nil {
		file.Write(data.Bytes())
		log.Println(err)
		return
	}

	file.Write(source)

	return
}

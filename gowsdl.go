// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package gowsdl

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"
)

const maxRecursion uint8 = 100

// GoWSDL defines the struct for WSDL generator.
type GoWSDL struct {
	loc                   *Location
	pkg                   string
	ignoreTLS             bool
	ignoreTypeNs          bool
	auth                  *basicAuth
	exportAllTypes        bool
	wsdl                  *WSDL
	resolvedXSDExternals  map[string]bool
	currentRecursionLevel uint8
	tmplFuncs             *tmplFunctions
}

var cacheDir = filepath.Join(os.TempDir(), "gowsdl-cache")

func init() {
	err := os.MkdirAll(cacheDir, 0700)
	if err != nil {
		log.Println("Create cache directory", "error", err)
		os.Exit(1)
	}
}

var timeout = time.Duration(30 * time.Second)

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, timeout)
}

func downloadFile(url string, ignoreTLS bool, auth *basicAuth) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: ignoreTLS,
		},
		Dial: dialTimeout,
	}
	client := &http.Client{Transport: tr}

	req, _ := http.NewRequest("GET", url, nil)
	if auth != nil {
		req.SetBasicAuth(auth.Login, auth.Password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("received response code %d", resp.StatusCode)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// NewGoWSDL initializes WSDL generator.
func NewGoWSDL(file, pkg string, ignoreTLS bool, exportAllTypes bool) (*GoWSDL, error) {
	file = strings.TrimSpace(file)
	if file == "" {
		return nil, errors.New("WSDL file is required to generate Go proxy")
	}

	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		pkg = "myservice"
	}

	r, err := ParseLocation(file)
	if err != nil {
		return nil, err
	}

	return &GoWSDL{
		loc:            r,
		pkg:            pkg,
		ignoreTLS:      ignoreTLS,
		exportAllTypes: exportAllTypes,
	}, nil
}

func (g *GoWSDL) SetBasicAuth(login, password string) {
	g.auth = &basicAuth{Login: login, Password: password}
}

func (g *GoWSDL) SetIgnoreTypeNamespaces(ignore bool) {
	g.ignoreTypeNs = ignore
}

// Start initiates the code generation process by starting two goroutines: one
// to generate types and another one to generate operations.
func (g *GoWSDL) Start() (map[string][]byte, error) {
	gocode := make(map[string][]byte)

	err := g.unmarshal()
	if err != nil {
		return nil, err
	}

	g.refineRawWsdlData()

	// Process WSDL nodes
	for _, schema := range g.wsdl.Types.Schemas {
		newTraverser(schema, g.wsdl.Types.Schemas).traverse()
	}

	g.tmplFuncs = createTmplFunctions(g)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error

		gocode["types"], err = g.genTypes()
		if err != nil {
			log.Println("genTypes", "error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error

		gocode["operations"], err = g.genOperations()
		if err != nil {
			log.Println(err)
		}
	}()

	wg.Wait()

	gocode["header"], err = g.genHeader()
	if err != nil {
		log.Println(err)
	}

	gocode["soap"], err = g.genSOAPClient()
	if err != nil {
		log.Println(err)
	}

	return gocode, nil
}

func (g *GoWSDL) fetchFile(loc *Location) (data []byte, err error) {
	if loc.f != "" {
		log.Println("[INFO] Reading", "file", loc.f)
		data, err = ioutil.ReadFile(loc.f)
	} else {
		log.Println("[INFO] Downloading", "file", loc.u.String())
		data, err = downloadFile(loc.u.String(), g.ignoreTLS, g.auth)
	}
	return
}

func (g *GoWSDL) unmarshal() error {
	data, err := g.fetchFile(g.loc)
	if err != nil {
		return err
	}

	g.wsdl = new(WSDL)
	if err = xml.Unmarshal(data, g.wsdl); err != nil {
		return err
	}

	g.resolvedXSDExternals = make(map[string]bool, maxRecursion)
	for _, schema := range g.wsdl.Types.Schemas {
		if err = g.resolveXSDExternals(schema, g.loc); err != nil {
			return err
		}
	}

	return nil
}

func (g *GoWSDL) resolveXSDExternals(schema *XSDSchema, loc *Location) error {
	if schema == nil || loc == nil {
		return nil
	}

	g.currentRecursionLevel++
	if g.currentRecursionLevel > maxRecursion {
		return nil
	}

	currentSchemaKey := loc.String()
	if g.resolvedXSDExternals[currentSchemaKey] {
		return nil
	}
	g.resolvedXSDExternals[currentSchemaKey] = true

	log.Printf("[INFO] Resolving external XSDs for Schema %s", currentSchemaKey)

	handleExternalSchema := func(base *Location, schemaLoc string) error {
		var (
			newSchema    *XSDSchema
			newSchemaLoc *Location
			err          error
		)
		if newSchema, newSchemaLoc, err = g.downloadSchemaIfRequired(loc, schemaLoc); err == nil && newSchema != nil {
			g.wsdl.Types.Schemas = append(g.wsdl.Types.Schemas, newSchema)
			err = g.resolveXSDExternals(newSchema, newSchemaLoc)
		}
		return err
	}

	var err error
	for _, impt := range schema.Imports {
		if err != nil {
			break
		}
		if impt.SchemaLocation == "" {
			log.Printf("[WARN] Don't know where to find XSD for %s", impt.Namespace)
			continue
		}
		err = handleExternalSchema(loc, impt.SchemaLocation)
	}
	for _, incl := range schema.Includes {
		if err != nil {
			break
		}
		if incl.SchemaLocation == "" {
			continue
		}
		err = handleExternalSchema(loc, incl.SchemaLocation)
	}
	return err
}

func (g *GoWSDL) downloadSchemaIfRequired(base *Location,
	locationRef string) (newSchema *XSDSchema,
	newSchemaLoc *Location,
	err error) {
	if newSchemaLoc, err = base.Parse(locationRef); err != nil {
		return
	}
	schemaKey := newSchemaLoc.String()
	if g.resolvedXSDExternals[schemaKey] {
		return
	}

	var data []byte
	if data, err = g.fetchFile(newSchemaLoc); err != nil {
		return
	}

	newSchema = new(XSDSchema)
	if err = xml.Unmarshal(data, newSchema); err != nil {
		return
	}

	log.Printf("[INFO] Downloaded Schema %s", newSchema.TargetNamespace)

	return
}

func (g *GoWSDL) refineRawWsdlData() {
	g.wsdl.refine(g.ignoreTypeNs)
}

func (g *GoWSDL) genTypes() ([]byte, error) {
	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("types").
		Funcs(g.tmplFuncs.funcMap).Parse(typesTmpl))
	err := tmpl.Execute(data, g.wsdl.Types)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genOperations() ([]byte, error) {
	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("operations").
		Funcs(g.tmplFuncs.funcMap).Parse(opsTmpl))
	err := tmpl.Execute(data, g.wsdl.PortTypes)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genHeader() ([]byte, error) {
	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("header").
		Funcs(g.tmplFuncs.funcMap).Parse(headerTmpl))
	err := tmpl.Execute(data, g.pkg)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

func (g *GoWSDL) genSOAPClient() ([]byte, error) {
	data := new(bytes.Buffer)
	tmpl := template.Must(template.New("soapclient").Parse(soapTmpl))
	err := tmpl.Execute(data, g.pkg)
	if err != nil {
		return nil, err
	}

	return data.Bytes(), nil
}

package gowsdl

import (
	"errors"
	"log"
	"strings"
	"text/template"
	"unicode"
)

type tmplFunctions struct {
	funcMap template.FuncMap
}

var reservedWords = map[string]string{
	"break":       "break_",
	"default":     "default_",
	"func":        "func_",
	"interface":   "interface_",
	"select":      "select_",
	"case":        "case_",
	"defer":       "defer_",
	"go":          "go_",
	"map":         "map_",
	"struct":      "struct_",
	"chan":        "chan_",
	"else":        "else_",
	"goto":        "goto_",
	"package":     "package_",
	"switch":      "switch_",
	"const":       "const_",
	"fallthrough": "fallthrough_",
	"if":          "if_",
	"range":       "range_",
	"type":        "type_",
	"continue":    "continue_",
	"for":         "for_",
	"import":      "import_",
	"return":      "return_",
	"var":         "var_",
}

var xsd2GoTypes = map[string]string{
	"string":        "string",
	"token":         "string",
	"float":         "float32",
	"double":        "float64",
	"decimal":       "float64",
	"integer":       "int32",
	"int":           "int32",
	"short":         "int16",
	"byte":          "int8",
	"long":          "int64",
	"boolean":       "bool",
	"datetime":      "time.Time",
	"date":          "time.Time",
	"time":          "time.Time",
	"base64binary":  "[]byte",
	"hexbinary":     "[]byte",
	"unsignedint":   "uint32",
	"unsignedshort": "uint16",
	"unsignedbyte":  "byte",
	"unsignedlong":  "uint64",
	"anytype":       "interface{}",
}

func createTmplFunctions(g *GoWSDL) *tmplFunctions {
	// Normalizes value to be used as a valid Go identifier, avoiding compilation issues
	normalize := func(value string) string {
		mapping := func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return r
			}
			return -1
		}
		return strings.Map(mapping, value)
	}

	replaceReservedWords := func(identifier string) string {
		value := reservedWords[identifier]
		if value != "" {
			return value
		}
		return normalize(identifier)
	}

	removeNS := func(xsdType string) string {
		// Handles name space, ie. xsd:string, xs:string
		r := strings.Split(xsdType, ":")
		if len(r) == 2 {
			return r[1]
		}
		return r[0]
	}

	toGoTypeNs := func(xsdType string, ns string) string {
		log.Printf("xsdType: %s, ns: %s", xsdType, ns)
		// Handles name space, ie. xsd:string, xs:string
		r := strings.Split(xsdType, ":")
		t := r[0]
		if len(r) == 2 {
			t = r[1]
		}

		value := xsd2GoTypes[strings.ToLower(t)]
		if value != "" {
			return value
		}

		if !g.ignoreTypeNs && ns != "" {
			t = ns + t
		}
		return "*" + replaceReservedWords(makePublic(t))
	}

	toGoType := func(xsdType string) string {
		return toGoTypeNs(xsdType, "")
	}

	// TODO(c4milo): Add namespace support instead of stripping it
	stripns := func(xsdType string) string {
		r := strings.Split(xsdType, ":")
		t := r[0]
		if len(r) == 2 {
			t = r[1]
		}
		return t
	}

	makePublic := func(identifier string) string {
		if !g.exportAllTypes {
			return identifier
		}
		return makePublic(identifier)
	}

	comment := func(text string) string {
		lines := strings.Split(text, "\n")

		var output string
		if len(lines) == 1 && lines[0] == "" {
			return ""
		}

		// Helps to determine if there is an actual comment without screwing newlines
		// in real comments.
		hasComment := false

		for _, line := range lines {
			line = strings.TrimLeftFunc(line, unicode.IsSpace)
			if line != "" {
				hasComment = true
			}
			output += "\n// " + line
		}

		if hasComment {
			return output
		}
		return ""
	}

	// Given a message, finds its type.
	//
	// I'm not very proud of this function but
	// it works for now and performance doesn't
	// seem critical at this point
	findType := func(message string) string {
		message = stripns(message)

		for _, msg := range g.wsdl.Messages {
			if msg.Name != message {
				continue
			}

			// Assumes document/literal wrapped WS-I
			if len(msg.Parts) == 0 {
				// Message does not have parts. This could be a Port
				// with HTTP binding or SOAP 1.2 binding, which are not currently
				// supported.
				log.Printf("[WARN] %s message doesn't have any parts, ignoring message...", msg.Name)
				continue
			}

			part := msg.Parts[0]
			if part.Type != "" {
				return stripns(part.Type)
			}

			elRef := stripns(part.Element)

			for _, schema := range g.wsdl.Types.Schemas {
				for _, el := range schema.Elements {
					if strings.EqualFold(elRef, el.Name) {
						if el.Type != "" {
							return stripns(el.Type)
						}
						return el.Name
					}
				}
			}
		}
		return ""
	}

	// TODO(c4milo): Add support for namespaces instead of striping them out
	// TODO(c4milo): improve runtime complexity if performance turns out to be an issue.
	findSOAPAction := func(operation, portType string) string {
		for _, binding := range g.wsdl.Binding {
			if stripns(binding.Type) != portType {
				continue
			}

			for _, soapOp := range binding.Operations {
				if soapOp.Name == operation {
					return soapOp.SOAPOperation.SOAPAction
				}
			}
		}
		return ""
	}

	findServiceAddress := func(name string) string {
		for _, service := range g.wsdl.Service {
			for _, port := range service.Ports {
				if port.Name == name {
					return port.SOAPAddress.Location
				}
			}
		}
		return ""
	}

	return &tmplFunctions{
		funcMap: map[string]interface{}{
			"normalize":            normalize,
			"replaceReservedWords": replaceReservedWords,
			"removeNS":             removeNS,
			"toGoTypeNs":           toGoTypeNs,
			"toGoType":             toGoType,
			"stripns":              stripns,
			"comment":              comment,
			"makePublic":           makePublic,
			"makeFieldPublic":      makePublic,
			"goString":             goString,
			"dict":                 dict,
			"findType":             findType,
			"findSOAPAction":       findSOAPAction,
			"findServiceAddress":   findServiceAddress,
		},
	}
}

func goString(s string) string {
	return strings.Replace(s, "\"", "\\\"", -1)
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	valuesCount := len(values)
	if valuesCount%2 != 0 {
		return nil, errors.New("[ERROR] Odd number of arguments, even expected")
	}
	resultDict := make(map[string]interface{}, valuesCount/2)
	for i := 0; i < valuesCount; i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("[ERROR] dict keys must be strings")
		}
		resultDict[key] = values[i+1]
	}
	return resultDict, nil
}

func makePublic(identifier string) string {
	field := []rune(identifier)
	if len(field) == 0 {
		return identifier
	}
	field[0] = unicode.ToUpper(field[0])
	return string(field)
}

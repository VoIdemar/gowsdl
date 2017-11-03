package gowsdl

func (w *WSDL) refine(ignoreTypeNs bool) {
	w.Types.removeTypeDuplicates(ignoreTypeNs)
}

func (wsdlType *WSDLType) removeTypeDuplicates(ignoreTypeNs bool) {
	handledTypesDict := make(map[string]bool)
	for _, schema := range wsdlType.Schemas {
		var uniqueSimpleTypes []*XSDSimpleType
		var fullTypeName string
		for _, simpleType := range schema.SimpleType {
			if fullTypeName = getFullTypeName(simpleType.Name, schema.TargetNamespace, ignoreTypeNs);
				!handledTypesDict[fullTypeName] {
				handledTypesDict[fullTypeName] = true
				uniqueSimpleTypes = append(uniqueSimpleTypes, simpleType)
			}
		}
		schema.SimpleType = uniqueSimpleTypes

		var uniqueComplexTypes []*XSDComplexType
		for _, complexType := range schema.ComplexTypes {
			if fullTypeName = getFullTypeName(complexType.Name, schema.TargetNamespace, ignoreTypeNs);
				!handledTypesDict[fullTypeName] {
				handledTypesDict[fullTypeName] = true
				uniqueComplexTypes = append(uniqueComplexTypes, complexType)
			}
		}
		schema.ComplexTypes = uniqueComplexTypes
	}
}

func getFullTypeName(typeName, ns string, ignoreTypeNs bool) string {
	name := typeName
	if !ignoreTypeNs {
		name = ns + ":" + typeName
	}
	return name
}

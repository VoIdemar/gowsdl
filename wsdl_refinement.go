package gowsdl

func (w *WSDL) refine() {
	w.Types.removeTypeDuplicates()
}

func (wsdlType *WSDLType) removeTypeDuplicates() {
	handledTypesDict := make(map[string]bool)
	for _, schema := range wsdlType.Schemas {
		var uniqueSimpleTypes []*XSDSimpleType
		var fullTypeName string
		for _, simpleType := range schema.SimpleType {
			if fullTypeName = schema.TargetNamespace + ":" + simpleType.Name;
				!handledTypesDict[fullTypeName] {
				handledTypesDict[fullTypeName] = true
				uniqueSimpleTypes = append(uniqueSimpleTypes, simpleType)
			}
		}
		schema.SimpleType = uniqueSimpleTypes

		var uniqueComplexTypes []*XSDComplexType
		for _, complexType := range schema.ComplexTypes {
			if fullTypeName = schema.TargetNamespace + ":" + complexType.Name;
				!handledTypesDict[fullTypeName] {
				handledTypesDict[fullTypeName] = true
				uniqueComplexTypes = append(uniqueComplexTypes, complexType)
			}
		}
		schema.ComplexTypes = uniqueComplexTypes
	}
}

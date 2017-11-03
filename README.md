# WSDL to Go

Generates Go code from a WSDL file.

This is a fork of the "[hooklift/gowsdl](https://github.com/hooklift/gowsdl)" library which adds a couple of features to the original lib:
* HTTP Basic Auth support
* Removal of the type duplicates (at least partially)
* XSDs from "includes" and "imports" are collected to the deepest level possible (original library stopped recursion if there were no "includes" in the current XSD + ignored some of them due to recursive algorithm implementation).

### Install

* Download and build locally: `go get github.com/VoIdemar/gowsdl/...`

Please refer to the README page of the original library for more details.

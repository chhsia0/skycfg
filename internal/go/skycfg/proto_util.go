// Copyright 2018 The Skycfg Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package skycfg

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strings"

	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"
	descriptor_pb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	"k8s.io/gengo/namer"
)

func mustParseFileDescriptor(gzBytes []byte) *descriptor_pb.FileDescriptorProto {
	gz, err := gzip.NewReader(bytes.NewReader(gzBytes))
	if err != nil {
		panic(fmt.Sprintf("EnumDescriptor: %v", err))
	}
	defer gz.Close()

	fileDescBytes, err := ioutil.ReadAll(gz)
	if err != nil {
		panic(fmt.Sprintf("EnumDescriptor: %v", err))
	}

	fileDesc := &descriptor_pb.FileDescriptorProto{}
	if err := proto.Unmarshal(fileDescBytes, fileDesc); err != nil {
		panic(fmt.Sprintf("EnumDescriptor: %v", err))
	}
	return fileDesc
}

func messageTypeName(msg proto.Message) string {
	if hasName, ok := msg.(interface {
		XXX_MessageName() string
	}); ok {
		return hasName.XXX_MessageName()
	}

	hasDesc, ok := msg.(descriptor.Message)
	if !ok {
		return proto.MessageName(msg)
	}

	gzBytes, path := hasDesc.Descriptor()
	fileDesc := mustParseFileDescriptor(gzBytes)
	var chunks []string
	if pkg := fileDesc.GetPackage(); pkg != "" {
		chunks = append(chunks, pkg)
	}

	msgDesc := fileDesc.MessageType[path[0]]
	for ii := 1; ii < len(path); ii++ {
		chunks = append(chunks, msgDesc.GetName())
		msgDesc = msgDesc.NestedType[path[ii]]
	}
	chunks = append(chunks, msgDesc.GetName())
	return strings.Join(chunks, ".")
}

// Wrapper around proto.GetProperties with a reflection-based fallback
// around oneof parsing for GoGo.
func protoGetProperties(t reflect.Type) *proto.StructProperties {
	got := proto.GetProperties(t)

	// Set the protobuf field names and tag numbers for untagged public
	// fields to support go-to-protobuf messages.
	highest := 0
	for _, prop := range got.Prop {
		if prop.Tag > highest {
			highest = prop.Tag
		}
	}
	log.Println(t)
	fields := []reflect.StructField{}
	for ii := 0; ii < t.NumField(); ii++ {
		f := t.Field(ii)
		if f.Tag.Get("protobuf") == "" && f.Tag.Get("protobuf_oneof") == "" &&
			!strings.HasPrefix(f.Name, "XXX_") && !namer.IsPrivateGoName(f.Name) {
			// Extract name from JSON field tag.
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "" {
				name = namer.IL(f.Name)
			}
			if name != "-" {
				var wire string
				switch f.Type.Kind() {
				case reflect.Bool, reflect.Int, reflect.Int32, reflect.Int64,
					reflect.Uint, reflect.Uint32, reflect.Uint64:
					wire = "varint"
				case reflect.Map, reflect.Ptr, reflect.Slice, reflect.String:
					wire = "bytes"
				case reflect.Float32:
					wire = "fixed32"
				case reflect.Float64:
					wire = "fixed64"
				}
				highest++
				f.Tag = reflect.StructTag(fmt.Sprintf(`protobuf:"%s,%d,name=%s"`, wire, highest, name))
			}
		}
		log.Println(f.Tag)
		fields = append(fields, f)
	}
	got.Prop = proto.GetProperties(reflect.StructOf(fields)).Prop
	for _, prop := range got.Prop {
		log.Println(prop)
	}

	// If OneofTypes was already populated, then the go-protobuf
	// properties parser was fully successful and we don't need to do
	// anything more.
	if len(got.OneofTypes) > 0 {
		return got
	}

	// If the oneofs map is empty, it might be because the message
	// contains no oneof fields. We also don't need to do anything.
	expectOneofs := false
	for ii := 0; ii < t.NumField(); ii++ {
		f := t.Field(ii)
		if f.Tag.Get("protobuf_oneof") != "" {
			expectOneofs = true
			break
		}
	}
	if !expectOneofs {
		return got
	}

	// proto.GetProperties will ignore oneofs for GoGo generated code,
	// even though the tags and structures are identical. This is a
	// side-effect of XXX_OneofFuncs() containing nominal interface types
	// in its signature, and can be worked around with reflection.
	msg := reflect.New(t)
	oneofFuncsFn := msg.MethodByName("XXX_OneofFuncs")
	if !oneofFuncsFn.IsValid() {
		return got
	}

	// proto.GetProperties returns a mutable pointer to global internal
	// state of the protobuf library. Avoid spooky behavior by doing a
	// shallow copy.
	got = &proto.StructProperties{
		Prop:       got.Prop,
		OneofTypes: make(map[string]*proto.OneofProperties),
	}

	// This will panic if the API of XXX_OneofFuncs() changes significantly.
	// Hopefully that won't happen before the go-protobuf v2 API makes this
	// workaround unnecessary.
	oneofFuncs := oneofFuncsFn.Call([]reflect.Value{})
	oneofTypes := oneofFuncs[len(oneofFuncs)-1].Interface().([]interface{})
	for _, oneofType := range oneofTypes {
		prop := &proto.OneofProperties{
			Type: reflect.ValueOf(oneofType).Type(),
			Prop: &proto.Properties{},
		}
		realField := prop.Type.Elem().Field(0)
		prop.Prop.Name = realField.Name
		prop.Prop.Parse(realField.Tag.Get("protobuf"))
		for ii := 0; ii < t.NumField(); ii++ {
			f := t.Field(ii)
			if prop.Type.AssignableTo(f.Type) {
				prop.Field = ii
				break
			}
		}
		got.OneofTypes[prop.Prop.OrigName] = prop
	}

	return got
}

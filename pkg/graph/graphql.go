/*
Copyright AppsCode Inc. and Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package graph

import (
	"fmt"

	"github.com/graphql-go/graphql"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/resource-metadata/hub"
)

func getGraphQLSchema() graphql.Schema {
	oidType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "ObjectID",
		Description: "Uniquely identifies a Kubernetes object",
		Fields: graphql.Fields{
			"group": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "The group of the Object",
			},
			"kind": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "The kind of the Object",
			},
			"namespace": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "The namespace of the Object",
			},
			"name": &graphql.Field{
				Type:        graphql.String,
				Description: "The name of the Object.",
			},
		},
	})
	for _, label := range hub.ListEdgeLabels() {
		func(edgeLabel kmapi.EdgeLabel) {
			oidType.AddFieldConfig(string(edgeLabel), &graphql.Field{
				Type:        graphql.NewList(oidType),
				Description: fmt.Sprintf("%s from this object", edgeLabel),
				Args: graphql.FieldConfigArgument{
					"group": &graphql.ArgumentConfig{
						Description: "group of the linked objects",
						Type:        graphql.String, // optional graphql.NewNonNull(graphql.String),
					},
					"kind": &graphql.ArgumentConfig{
						Description: "kind of the linked objects",
						Type:        graphql.String,
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					var group, kind string
					if v, ok := p.Args["group"]; ok {
						group = v.(string)
					}
					if v, ok := p.Args["kind"]; ok {
						kind = v.(string)
					}
					if group != "" && kind == "" { // group can be empty
						return nil, fmt.Errorf("group is set but kind is not set")
					}

					if oid, ok := p.Source.(kmapi.ObjectID); ok {
						links, err := objGraph.Links(&oid, edgeLabel)
						if err != nil {
							return nil, err
						}
						if kind != "" { // group can be empty
							linksForGK := links[metav1.GroupKind{Group: group, Kind: kind}]
							return linksForGK, nil
						}

						var out []kmapi.ObjectID
						for _, refs := range links {
							out = append(out, refs...)
						}
						return out, nil
					}
					return []interface{}{}, nil
				},
			})
		}(label)
	}

	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"find": &graphql.Field{
				Type: oidType,
				Args: graphql.FieldConfigArgument{
					"oid": &graphql.ArgumentConfig{
						Description: "Object ID in OID format",
						Type:        graphql.NewNonNull(graphql.String),
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					key := p.Args["oid"].(string)
					oid, err := kmapi.ParseObjectID(kmapi.OID(key))
					if err != nil {
						return nil, err
					}
					return *oid, nil
				},
			},
		},
	})
	schema, _ := graphql.NewSchema(graphql.SchemaConfig{
		Query: queryType,
	})
	return schema
}

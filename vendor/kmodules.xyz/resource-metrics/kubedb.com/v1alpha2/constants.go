/*
Copyright AppsCode Inc. and Contributors

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

package v1alpha2

const (
	DBModeEnsemble    = "Ensemble"
	DBModeCluster     = "Cluster"
	DBModeSharded     = "Sharded"
	DBModeStandalone  = "Standalone"
	DBModeDistributed = "Distributed"
	DBModeReplicaSet  = "ReplicaSet"
	DBModeDedicated   = "Dedicated"
	DBModeCombined    = "Combined"

	DBModePrimaryOnly = "PrimaryOnly"
)

const (
	CassandraContainerName   = "cassandra"
	ClickHouseContainerName  = "clickhouse"
	DruidContainerName       = "druid"
	HazelcastContainerName   = "hazelcast"
	HanaDBContainerName      = "hanadb"
	FerretDBContainerName    = "ferretdb"
	IgniteContainerName      = "ignite"
	MSSQLServerContainerName = "mssql"
	Neo4jContainerName       = "neo4j"
	OracleContainerName      = "oracle"
	PgpoolContainerName      = "pgpool"
	QdrantContainerName      = "qdrant"
	RabbitMQContainerName    = "rabbitmq"
	SinglestoreContainerName = "singlestore"
	SolrContainerName        = "solr"
	ZooKeeperContainerName   = "zookeeper"

	SinglestoreSidecarContainerName = "singlestore-coordinator"
	MSSQLServerSidecarContainerName = "mssql-coordinator"
	HanaDBCoordinatorContainerName  = "hanadb-coordinator"
	OracleSidecarContainerName      = "oracle-coordinator"
	OracleObserverContainerName     = "observer"
)

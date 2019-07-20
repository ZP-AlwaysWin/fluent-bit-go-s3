package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/stretchr/testify/assert"
)

func TestCreateJSON(t *testing.T) {
	record := make(map[interface{}]interface{})
	record["key"] = "value"
	record["number"] = 8

	line, err := createJSON(record)
	if err != nil {
		assert.Fail(t, "createJSON fails:%v", err)
	}
	assert.NotNil(t, line, "json string not to be nil")
	result := make(map[string]interface{})
	jsonBytes := ([]byte)(line)
	err = json.Unmarshal(jsonBytes, &result)
	if err != nil {
		assert.Fail(t, "unmarshal of json fails:%v", err)
	}

	assert.Equal(t, result["key"], "value")
	assert.Equal(t, result["number"], float64(8))
}

func TestGenerateObjectKey(t *testing.T) {
	now := time.Now()
	objectKey := GenerateObjectKey("s3exampleprefix", now)
	fmt.Printf("objectKey: %v\n", objectKey)
	assert.NotNil(t, objectKey, "objectKey not to be nil")
}

type testrecord struct {
	rc   int
	ts   interface{}
	data map[interface{}]interface{}
}

type events struct {
	data []byte
}
type testFluentPlugin struct {
	credential      string
	accessKeyID     string
	secretAccessKey string
	bucket          string
	s3prefix        string
	region          string
	records         []testrecord
	position        int
	events          []*events
}

func (p *testFluentPlugin) PluginConfigKey(ctx unsafe.Pointer, key string) string {
	switch key {
	case "Credential":
		return p.credential
	case "AccessKeyID":
		return p.accessKeyID
	case "SecretAccessKey":
		return p.secretAccessKey
	case "Bucket":
		return p.bucket
	case "S3Prefix":
		return p.s3prefix
	case "Region":
		return p.region
	}
	return "unknown-" + key
}

func (p *testFluentPlugin) Unregister(ctx unsafe.Pointer) {}
func (p *testFluentPlugin) GetRecord(dec *output.FLBDecoder) (int, interface{}, map[interface{}]interface{}) {
	if p.position < len(p.records) {
		r := p.records[p.position]
		p.position++
		return r.rc, r.ts, r.data
	}
	return -1, nil, nil
}
func (p *testFluentPlugin) NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder { return nil }
func (p *testFluentPlugin) Exit(code int)                                                 {}
func (p *testFluentPlugin) Put(objectKey string, timestamp time.Time, line string) error {
	data := ([]byte)(line)
	events := &events{data: data}
	p.events = append(p.events, events)
	return nil
}
func (p *testFluentPlugin) addrecord(rc int, ts interface{}, line map[interface{}]interface{}) {
	p.records = append(p.records, testrecord{rc: rc, ts: ts, data: line})
}

type stubProvider struct {
	creds   credentials.Value
	expired bool
	err     error
}

func (s *stubProvider) Retrieve() (credentials.Value, error) {
	s.expired = false
	s.creds.ProviderName = "stubProvider"
	return s.creds, s.err
}
func (s *stubProvider) IsExpired() bool {
	return s.expired
}

type testS3Credential struct {
	credential string
}

func (c *testS3Credential) GetCredentials(accessID, secretkey, credential string) (*credentials.Credentials, error) {
	creds := credentials.NewCredentials(&stubProvider{
		creds: credentials.Value{
			AccessKeyID:     "AKID",
			SecretAccessKey: "SECRET",
			SessionToken:    "",
		},
		expired: true,
	})

	return creds, nil
}

func TestPluginInitializationWithStaticCredentials(t *testing.T) {
	s3Creds = &testS3Credential{}
	_, err := getS3Config("exampleaccessID", "examplesecretkey", "", "exampleprefix", "examplebucket", "exampleregion")
	if err != nil {
		t.Fatalf("failed test %#v", err)
	}
	plugin = &testFluentPlugin{
		accessKeyID:     "exampleaccesskeyid",
		secretAccessKey: "examplesecretaccesskey",
		bucket:          "examplebucket",
		s3prefix:        "exampleprefix",
		region:          "exampleregion",
	}
	res := FLBPluginInit(nil)
	assert.Equal(t, output.FLB_OK, res)
}

func TestPluginInitializationWithSharedCredentials(t *testing.T) {
	s3Creds = &testS3Credential{}
	_, err := getS3Config("", "", "examplecredentials", "exampleprefix", "examplebucket", "exampleregion")
	if err != nil {
		t.Fatalf("failed test %#v", err)
	}
	plugin = &testFluentPlugin{
		credential: "examplecredentials",
		bucket:     "examplebucket",
		s3prefix:   "exampleprefix",
		region:     "exampleregion",
	}
	res := FLBPluginInit(nil)
	assert.Equal(t, output.FLB_OK, res)
}

func TestPluginFlusher(t *testing.T) {
	testplugin := &testFluentPlugin{
		credential:      "examplecredentials",
		accessKeyID:     "exampleaccesskeyid",
		secretAccessKey: "examplesecretaccesskey",
		bucket:          "examplebucket",
		s3prefix:        "exampleprefix",
	}
	ts := time.Date(2019, time.March, 10, 10, 11, 12, 0, time.UTC)
	testrecords := map[interface{}]interface{}{
		"mykey": "myvalue",
	}
	testplugin.addrecord(0, output.FLBTime{Time: ts}, testrecords)
	testplugin.addrecord(0, uint64(ts.Unix()), testrecords)
	testplugin.addrecord(0, 0, testrecords)
	plugin = testplugin
	res := FLBPluginFlush(nil, 0, nil)
	assert.Equal(t, output.FLB_OK, res)
	assert.Len(t, testplugin.events, 1) // event length should be 1.
	var parsed map[string]interface{}
	json.Unmarshal(testplugin.events[0].data, &parsed)
	expected := `{"mykey":"myvalue"}
{"mykey":"myvalue"}
{"mykey":"myvalue"}
`
	assert.Equal(t, expected, string(testplugin.events[0].data))
}

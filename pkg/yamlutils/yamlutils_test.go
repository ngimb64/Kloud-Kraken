package yamlutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestUpdateYAMLBytes_CreateFromEmpty(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    // Define updates to be added to empty YAML
    updates := map[string]string{
        "aws_env.region": "us-east-2",
        "vpc.id":         "vpc-abcdef12",
    }

    // Run the function
    out, err := UpdateYAMLBytes([]byte{}, updates)

    // Validate results
    assert.Equal(nil, err)
    assert.NotEmpty(out)

    // Parse YAML to verify structure
    var doc map[string]interface{}
    err = yaml.Unmarshal(out, &doc)
    assert.Equal(nil, err)

    // Validate aws_env.region
    awsEnv, ok := doc["aws_env"].(map[string]interface{})
    assert.Equal(true, ok)
    assert.Equal("us-east-2", awsEnv["region"])

    // Validate vpc.id
    vpc, ok := doc["vpc"].(map[string]interface{})
    assert.Equal(true, ok)
    assert.Equal("vpc-abcdef12", vpc["id"])
}


func TestUpdateYAMLBytes_UpdateExisting(t *testing.T) {
    // Make reusable assert instance
    assert := assert.New(t)

    existing := []byte(`
aws_env:
  region: us-east-1
vpc:
  id: vpc-old
`)

    updates := map[string]string{
        "aws_env.region": "us-east-2",
        "vpc.name":       "kloud-kraken-vpc",
    }

    // Run the function
    out, err := UpdateYAMLBytes(existing, updates)

    // Validate results
    assert.Equal(nil, err)
    assert.NotEmpty(out)

    // Parse YAML to verify structure
    var doc map[string]interface{}
    err = yaml.Unmarshal(out, &doc)
    assert.Equal(nil, err)

    // Validate aws_env.region updated
    awsEnv, ok := doc["aws_env"].(map[string]interface{})
    assert.Equal(true, ok)
    assert.Equal("us-east-2", awsEnv["region"])

    // Validate vpc.id remains and vpc.name added
    vpc, ok := doc["vpc"].(map[string]interface{})
    assert.Equal(true, ok)
    assert.Equal("vpc-old", vpc["id"])
    assert.Equal("kloud-kraken-vpc", vpc["name"])
}

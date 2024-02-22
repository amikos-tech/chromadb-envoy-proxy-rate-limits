package chroma_quotas

import rego.v1

# Define maximum allowed values for each key
max_values := {
	"metadata_key_lengths": 10,
	"metadata_value_lengths": 10,
	"document_lengths": 20,
	"embeddings_dimensions": 5,
}

default validate := {
	"message": "ok",
	"allow": true,
}

# Main rule to validate the input data and generate messages
validate := result if {
	results := [x | some key, value in input; cnt := count([k | some k in value; k > max_values[key]]); cnt > 0; x := sprintf("%v key has %d values exceeding the maximum allowed value of %v", [key, cnt, max_values[key]])]
	count(results) > 0
	result := {
		"message": concat(", ", results),
		"allow": false,
	}
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tidwall/gjson"
	"log"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

func main() {
	// SetVMContext is the entrypoint for setting up this entire Wasm VM.
	// Please make sure that this entrypoint be called during "main()" function, otherwise
	// this VM would fail.
	proxywasm.SetVMContext(&vmContext{})
}

// vmContext implements types.VMContext interface of proxy-wasm-go SDK.
type vmContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

// Override types.DefaultVMContext.
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

// pluginContext implements types.PluginContext interface of proxy-wasm-go SDK.
type pluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext
	configuration pluginConfiguration
}

// pluginConfiguration is a type to represent an example configuration for this wasm plugin.
type pluginConfiguration struct {
	// Example configuration field.
	// The plugin will validate if those fields exist in the json payload.
	requiredKeys []string
}

// OnPluginStart Override types.DefaultPluginContext.
func (ctx *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	data, err := proxywasm.GetPluginConfiguration()
	if err != nil && !errors.Is(err, types.ErrorStatusNotFound) {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}
	config, err := parsePluginConfiguration(data)
	if err != nil {
		proxywasm.LogCriticalf("error parsing plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}
	ctx.configuration = config
	return types.OnPluginStartStatusOK
}

// parsePluginConfiguration parses the json plugin confiuration data and returns pluginConfiguration.
// Note that this parses the json data by gjson, since TinyGo doesn't support encoding/json.
// You can also try https://github.com/mailru/easyjson, which supports decoding to a struct.
func parsePluginConfiguration(data []byte) (pluginConfiguration, error) {
	if len(data) == 0 {
		return pluginConfiguration{}, nil
	}

	config := &pluginConfiguration{}
	if !gjson.ValidBytes(data) {
		return pluginConfiguration{}, fmt.Errorf("the plugin configuration is not a valid json: %q", string(data))
	}

	jsonData := gjson.ParseBytes(data)
	requiredKeys := jsonData.Get("requiredKeys").Array()
	for _, requiredKey := range requiredKeys {
		config.requiredKeys = append(config.requiredKeys, requiredKey.Str)
	}

	return *config, nil
}

// Override types.DefaultPluginContext.
func (ctx *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &payloadValidationContext{requiredKeys: ctx.configuration.requiredKeys, contextID: contextID}
}

// payloadValidationContext implements types.HttpContext interface of proxy-wasm-go SDK.
type payloadValidationContext struct {
	// Embed the default root http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext
	contextID            uint32
	totalRequestBodySize int
	requiredKeys         []string
}

//var _ types.HttpContext = (*payloadValidationContext)(nil)

// Override types.DefaultHttpContext.
func (*payloadValidationContext) OnHttpRequestHeaders(numHeaders int, _ bool) types.Action {
	contentType, err := proxywasm.GetHttpRequestHeader("content-type")
	if err != nil || contentType != "application/json" {
		// If the header doesn't have the expected content value, send the 403 response,
		if err := proxywasm.SendHttpResponse(403, nil, []byte("content-type must be provided"), -1); err != nil {
			return types.ActionContinue
		}
		// and terminates the further processing of this traffic by ActionPause.
		return types.ActionPause
	}

	// ActionContinue lets the host continue the processing the body.
	return types.ActionContinue
}

// OnHttpRequestBody Override types.DefaultHttpContext.
func (ctx *payloadValidationContext) OnHttpRequestBody(bodySize int, endOfStream bool) types.Action {
	ctx.totalRequestBodySize += bodySize
	if !endOfStream {
		// OnHttpRequestBody may be called each time a part of the body is received.
		// Wait until we see the entire body to replace.
		return types.ActionPause
	}

	body, err := proxywasm.GetHttpRequestBody(0, ctx.totalRequestBodySize)
	if err != nil {
		proxywasm.LogErrorf("failed to get request body: %v", err)
		return types.ActionContinue
	}

	if !gjson.ValidBytes(body) {
		proxywasm.LogErrorf("body is not a valid json: %q", string(body))
		return types.ActionContinue
	}
	jsonData := gjson.ParseBytes(body)
	for _, requiredKey := range ctx.requiredKeys {
		if !jsonData.Get(requiredKey).Exists() {
			proxywasm.LogErrorf("required key (%v) is missing: %v", requiredKey, jsonData)
			if err := proxywasm.SendHttpResponse(403, nil, []byte("Missing key in JSON payload."), -1); err != nil {
				return types.ActionContinue
			}
			return types.ActionContinue
		}
	}

	var res = processChromaBody(jsonData.Raw)
	jsonBytes, err := json.Marshal(res)
	if err != nil {
		log.Fatalf("Error marshaling to JSON: %v", err)
		return types.ActionContinue
	}

	// Make an HTTP request to the OPA server
	headers := [][2]string{
		{":method", "POST"}, {":authority", "some_authority"},
		{"accept", "*/*"},
		{":path", "/v1/data/chroma_quotas/validate"},
		{"content-type", "application/json"},
	}
	httpCallResponseCallback := func(numHeaders, bodySize, numTrailers int) {
		proxywasm.LogInfof("Dispatched ...")

		responseBody, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
		if err != nil {
			proxywasm.LogErrorf("failed to get response body: %v", err)
			return
		}

		isApproved := gjson.GetBytes(responseBody, "result.allow").Bool()
		if isApproved {
			// If approved, resume the HTTP request to continue processing
			err := proxywasm.ResumeHttpRequest()
			if err != nil {
				proxywasm.LogErrorf("failed to resume HTTP request: %v", err)
				return
			}
		} else {
			// If not approved, respond with an error and terminate the request
			if err := proxywasm.SendHttpResponse(429, nil, []byte("Quota exceeded"), -1); err != nil {
				proxywasm.LogErrorf("failed to send the 429 response: %v", err)
			}
			// Here, you might not need to explicitly pause or terminate since you're sending a response directly
		}
	}
	if _, err = proxywasm.DispatchHttpCall(
		"opa_service",
		headers,
		jsonBytes, nil, 2000, httpCallResponseCallback); err != nil {
		proxywasm.LogErrorf("dispatch httpcall failed: %v, %v", err, res)
		if err := proxywasm.SendHttpResponse(403, nil, []byte("Authorization failed."), -1); err != nil {
			return types.ActionContinue
		}
	}
	return types.ActionPause
}

// Function to process the JSON payload using gjson
func processChromaBody(jsonPayload string) map[string]interface{} {
	result := make(map[string][]int)

	// Parse and calculate metadata key and value lengths
	gjson.Get(jsonPayload, "metadatas").ForEach(func(key, value gjson.Result) bool {
		value.ForEach(func(subKey, subValue gjson.Result) bool {
			//check if subkey is a string
			if subKey.Type.String() == "String" {
				result["metadata_key_lengths"] = append(result["metadata_key_lengths"], len(subKey.String()))
			}
			if subValue.Type.String() == "String" {
				result["metadata_value_lengths"] = append(result["metadata_value_lengths"], len(subValue.String()))
			}
			return true // keep iterating
		})
		return true // keep iterating
	})

	// Calculate document lengths
	gjson.Get(jsonPayload, "documents").ForEach(func(_, value gjson.Result) bool {
		result["document_lengths"] = append(result["document_lengths"], len(value.String()))
		return true // keep iterating
	})

	// Calculate embeddings dimensions
	gjson.Get(jsonPayload, "embeddings").ForEach(func(_, value gjson.Result) bool {
		result["embeddings_dimensions"] = append(result["embeddings_dimensions"], len(value.Array()))
		return true // keep iterating
	})

	finalResult := make(map[string]interface{})
	finalResult["input"] = result
	return finalResult
}

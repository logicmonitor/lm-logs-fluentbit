//go:build linux || darwin || windows
// +build linux darwin windows

package main

import (
	"C"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"
	"unsafe"

	"github.com/fluent/fluent-bit-go/output"
	jsoniter "github.com/json-iterator/go"
	"github.com/logicmonitor/lm-data-sdk-go/api/logs"
)

const (
	outputDescription = "This is a fluent-bit output plugin that sends data to Logicmonitor"
	outputName        = "logicmonitor"
)

var (
	plugin Plugin = &bitPlugin{}
	logger        = NewLogger(outputName, true)
)

type LogicmonitorOutput struct {
	plugin      Plugin
	logger      *Logger
	client      *LogicmonitorClient
	logIngestor *logs.LMLogIngest
	id          string
}

var (
	outputs map[string]LogicmonitorOutput
)

// Plugin interface
type Plugin interface {
	Environment(ctx unsafe.Pointer, key string) string
	Unregister(ctx unsafe.Pointer)
	GetRecord(dec *output.FLBDecoder) (ret int, ts interface{}, rec map[interface{}]interface{})
	NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder
	Send(values []byte, client *LogicmonitorClient, logIngestor *logs.LMLogIngest) int
	Flush(*LogicmonitorClient, *logs.LMLogIngest) int
}

type bitPlugin struct{}

func (p *bitPlugin) Environment(ctx unsafe.Pointer, key string) string {
	return output.FLBPluginConfigKey(ctx, key)
}

func (p *bitPlugin) Unregister(ctx unsafe.Pointer) {
	output.FLBPluginUnregister(ctx)
}

func (p *bitPlugin) GetRecord(dec *output.FLBDecoder) (ret int, ts interface{}, rec map[interface{}]interface{}) {
	return output.GetRecord(dec)
}

func (p *bitPlugin) NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder {
	return output.NewDecoder(data, length)
}

func (p *bitPlugin) Send(log []byte, client *LogicmonitorClient, logIngestor *logs.LMLogIngest) int {
	return client.Send(log, logIngestor)
}

func (p *bitPlugin) Flush(client *LogicmonitorClient, logIngestor *logs.LMLogIngest) int {
	return client.Flush(logIngestor)
}

// FLBPluginRegister When Fluent Bit loads a Golang plugin,
// it looks up and loads the registration callback that aims
// to populate the internal structure with plugin name and description.
// This function is invoked at start time before any configuration is done inside the engine.
//
//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, outputName, outputDescription)
}

// FLBPluginInit Before the engine starts,
// it initializes all plugins that were configured.
// As part of the initialization, the plugin can obtain configuration parameters and do any other internal checks.
// It can also set the context for this instance in case params need to be retrieved during flush.
// The function must return FLB_OK when it initialized properly or FLB_ERROR if something went wrong.
// If the plugin reports an error, the engine will not load the instance.
//
//export FLBPluginInit
func FLBPluginInit(ctx unsafe.Pointer) int {
	if ctx != nil {
		if err := initConfigParams(ctx); err != nil {
			logger.Log(fmt.Sprintf("failed to initialize output configuration: %v", err))
			plugin.Unregister(ctx)
			return output.FLB_ERROR
		}

		output.FLBPluginSetContext(ctx, output.FLBPluginConfigKey(ctx, "id"))
	} else {
		return output.FLB_ERROR
	}
	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	return output.FLB_OK
}

// FLBPluginFlush Upon flush time, when Fluent Bit wants to flush it's buffers,
// the runtime flush callback will be triggered.
// The callback will receive the configuration context,
// a raw buffer of msgpack data,
// the proper bytes length and the associated tag.
// When done, there are three returning values available: FLB_OK, FLB_ERROR, FLB_RETRY.
//
//export FLBPluginFlushCtx
func FLBPluginFlushCtx(ctx, data unsafe.Pointer, length C.int, tag *C.char) int {
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}
	var id string
	if ctx != nil {
		id = output.FLBPluginGetContext(ctx).(string)
	}

	if id == "" {
		id = defaultId
	}

	logger.Debug(fmt.Sprintf("Flushing for id: %s", id))
	dec := plugin.NewDecoder(data, int(length))

	// Iterate Records
	for {
		// Extract Record
		ret, ts, record = plugin.GetRecord(dec)
		if ret != 0 {
			break
		}

		log, err := serializeRecord(ts, C.GoString(tag), record, id)
		if err != nil {
			continue
		}
		plugin.Send(log, outputs[id].client, outputs[id].logIngestor)
	}

	return plugin.Flush(outputs[id].client, outputs[id].logIngestor)
}

// FLBPluginExit When Fluent Bit will stop using the instance of the plugin,
// it will trigger the exit callback.
//
//export FLBPluginExit
func FLBPluginExit() int {
	plugin.Flush(nil, nil)
	return output.FLB_OK
}

func initConfigParams(ctx unsafe.Pointer) error {
	debug, err := strconv.ParseBool(output.FLBPluginConfigKey(ctx, "lmDebug"))
	if err != nil {
		debug = false
	}

	outputId := output.FLBPluginConfigKey(ctx, "id")

	if outputs == nil {
		outputs = make(map[string]LogicmonitorOutput)
	}

	if outputId == "" {
		logger.Debug(fmt.Sprintf("using default id: %s", defaultId))
		outputId = defaultId
	}

	if _, ok := outputs[outputId]; ok {
		logger.Log(fmt.Sprintf("output_id %s already exists, overriding", outputId))
	}

	logger = NewLogger(outputName+"_"+outputId, debug)

	lmCompanyName := output.FLBPluginConfigKey(ctx, "lmCompanyName")
	if lmCompanyName == "" {
		return fmt.Errorf("LM Company name is not specified. Please specify the company name in the configuration")
	}

	accessKey := output.FLBPluginConfigKey(ctx, "accessKey")
	accessID := output.FLBPluginConfigKey(ctx, "accessID")

	useBearerTokenforAuth := false
	if accessID == "" || accessKey == "" {
		logger.Log("accessID or accessKey is empty. Using bearer Token for authentication")
		useBearerTokenforAuth = true
	}
	bearerToken := output.FLBPluginConfigKey(ctx, "bearerToken")

	if accessID == "" || accessKey == "" {
		if bearerToken == "" {
			return fmt.Errorf("Bearer token not specified. Either access_id and access_key both or bearer_token must be specified for authentication with Logicmonitor.")
		}
	}

	resourceMapping := output.FLBPluginConfigKey(ctx, "resourceMapping")

	includeMetadata, err := strconv.ParseBool(output.FLBPluginConfigKey(ctx, "includeMetadata"))
	logSource := output.FLBPluginConfigKey(ctx, "logSource")
  if logSource == "" {
      logSource = "lm-logs-fluentbit"
  }
	versionId := output.FLBPluginConfigKey(ctx, "versionId")
  if versionId == "" {
      versionId = "1.0.0"
  }

	client := NewClient(lmCompanyName, accessID, accessKey, bearerToken, useBearerTokenforAuth, resourceMapping, includeMetadata, logSource, versionId, logger)
	logIngestor := NewLogIngester(client)
	outputs[outputId] = LogicmonitorOutput{
		logger:      logger,
		client:      client,
		logIngestor: logIngestor,
		id:          outputId,
	}

	return nil
}

func serializeRecord(ts interface{}, tag string, record map[interface{}]interface{}, outputId string) ([]byte, error) {
	body := parseJSON(record)
	var err error

	if _, ok := body["host"]; !ok {
		// Get hostname
		hostname, err := os.Hostname()
		if err != nil {
			hostname = "localhost"
		}
		body["host"] = hostname
	}

	body["timestamp"] = formatTimestamp(ts)
	body["fluentbit_tag"] = tag
	body["_resource.type"] = "Fluentbit"

	serialized, err := jsoniter.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to convert %+v to JSON: %v", record, err)
	}

	return serialized, nil
}

func parseJSON(record map[interface{}]interface{}) map[string]interface{} {
	jsonRecord := make(map[string]interface{})

	for k, v := range record {
		stringKey := k.(string)

		switch t := v.(type) {
		case []byte:
			// prevent encoding to base64
			jsonRecord[stringKey] = string(t)
		case map[interface{}]interface{}:
			jsonRecord[stringKey] = parseJSON(t)
		case []interface{}:
			var array []interface{}
			for _, e := range v.([]interface{}) {
				switch t := e.(type) {
				case []byte:
					array = append(array, string(t))
				case map[interface{}]interface{}:

					array = append(array, parseJSON(t))
				default:
					array = append(array, e)
				}
			}
			jsonRecord[stringKey] = array
		default:
			jsonRecord[stringKey] = v
		}
	}
	return jsonRecord
}
func formatTimestamp(ts interface{}) time.Time {
	var timestamp time.Time

	switch t := ts.(type) {
	case output.FLBTime:
		timestamp = ts.(output.FLBTime).Time
	case uint64:
		timestamp = time.Unix(int64(t), 0)
	case time.Time:
		timestamp = ts.(time.Time)
	case []interface{}:
		s := reflect.ValueOf(t)
		if s.Kind() != reflect.Slice || s.Len() < 2 {
			// Expects a non-empty slice of length 2, so we won't extract a timestamp.
			timestamp = formatTimestamp(s)
			return timestamp
		}
		ts = s.Index(0).Interface() // First item is the timestamp.
		timestamp = formatTimestamp(ts)
	default:
		logger.Log(fmt.Sprintf("Unknown format, defaulting to now, timestamp: %v of type: %T.\n", t, t))
		timestamp = time.Now()
	}
	return timestamp
}

func main() {
}

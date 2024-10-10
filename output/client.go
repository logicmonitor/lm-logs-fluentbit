//go:build linux || darwin || windows
// +build linux darwin windows

package main

import (
	"fmt"
	"github.com/fluent/fluent-bit-go/output"
	"context"
	"encoding/json"
	"github.com/logicmonitor/lm-data-sdk-go/api/logs"
	"github.com/logicmonitor/lm-data-sdk-go/model"
	"github.com/logicmonitor/lm-data-sdk-go/utils"
)

const (
	defaultURL                = "https://listener.lm.output:8071"
	defaultId                 = "lm_output_1"
	maxRequestBodySizeInBytes = 8 * 1024 * 1024 // 8MB
	megaByte                  = 1 * 1024 * 1024 // 1MB
)

// LogicmonitorClient http client that sends bulks to Logicmonitor http listener
type LogicmonitorClient struct {
	lmCompanyName          string
	accessID              string
	accessKey 			 string
	bearerToken string
	useBearerTokenforAuth bool
	resourceMapping		string
	bulk                 []model.LogInput
	includeMetadata bool
	logger               *Logger
	sizeThresholdInBytes int
}

// ClientOptionFunc options for Logicmonitor
type ClientOptionFunc func(*LogicmonitorClient) error

// NewClient is a constructor for Logicmonitor http client
func NewClient( lmCompanyName string, accessID string, accessKey string, bearerToken string, useBearerTokenforAuth bool, resourceMapping string, includeMetadata bool, logger *Logger ) (*LogicmonitorClient) {
	
	logicmonitorClient := &LogicmonitorClient{
		lmCompanyName:          lmCompanyName,
		accessID:              accessID,
		accessKey: 			 accessKey,
		bearerToken: bearerToken,
		useBearerTokenforAuth: useBearerTokenforAuth,
		resourceMapping:		resourceMapping,
		bulk: nil,
		includeMetadata: includeMetadata,
		logger: logger,
		sizeThresholdInBytes: maxRequestBodySizeInBytes,
	}
	
	return logicmonitorClient
	
	
}

func NewLogIngester(logicmonitorClient *LogicmonitorClient) (*logs.LMLogIngest){
	auth := utils.AuthParams{AccessID: logicmonitorClient.accessID,
		AccessKey:            logicmonitorClient.accessKey,
		BearerToken:          logicmonitorClient.bearerToken}

	companyName := logicmonitorClient.lmCompanyName
	options := []logs.Option{
		logs.WithLogBatchingDisabled(),
		logs.WithAuthentication(auth),
		logs.WithEndpoint("https://"+companyName+"/rest"),
		logs.WithUserAgent("lm-logs-fluentbit"),
	}

	lmLog, err := logs.NewLMLogIngest(context.Background(), options...)
	if err != nil {
		logger.Log(fmt.Sprintf("Error in initializing log ingest %s\n", err))
	}

	logger.Log("Log ingest initialized")
	return lmLog
}

// SetDebug mode and send logs to this writer
func SetDebug(debug bool) ClientOptionFunc {
	return func(logicmonitorClient *LogicmonitorClient) error {
		logicmonitorClient.logger.SetDebug(debug)
		logicmonitorClient.logger.Debug(fmt.Sprintf("Setting debug to %t\n", debug))
		return nil
	}
}

// SetBodySizeThreshold set the maximum body size of the client http request
// The param is in MB and can be between 0(mostly for testing) and 9
func SetBodySizeThreshold(threshold int) ClientOptionFunc {
	return func(logicmonitorClient *LogicmonitorClient) error {
		logicmonitorClient.sizeThresholdInBytes = threshold * megaByte
		if threshold < 0 || threshold > 9 {
			logicmonitorClient.logger.Debug("Falling back to the default BodySizeThreshold")
			logicmonitorClient.sizeThresholdInBytes = maxRequestBodySizeInBytes
		}
		logicmonitorClient.logger.Debug(fmt.Sprintf("Setting BodySizeThreshold to %d\n", logicmonitorClient.sizeThresholdInBytes))
		return nil
	}
}

// Send adds the log to the client bulk slice check if we should send the bulk
func (logicmonitorClient *LogicmonitorClient) Send(log []byte, logIngestor *logs.LMLogIngest) int {

	if (len(logicmonitorClient.bulk) + len(log) + 1) > logicmonitorClient.sizeThresholdInBytes {
		res := logicmonitorClient.sendBulk(logIngestor)
		logicmonitorClient.bulk = nil
		if res != output.FLB_OK {
			return res
		}
	}

  logger.Debug(fmt.Sprintf("Log received : %s", string(log)))

	jsonMap := make(map[string]interface{})
	json.Unmarshal(log, &jsonMap)
	resourceMap := make(map[string]interface{})
	metadata :=make(map[string]interface{})
	resourceMapReceived := make(map[string]string)
	err := json.Unmarshal([]byte(logicmonitorClient.resourceMapping), &resourceMapReceived)
	if err != nil {
		logger.Log(fmt.Sprintf("err for unmarshal resourceMapStr %s",err))
	 }

	for k, v := range resourceMapReceived {
		if(k != ""){
			resourceMap[v] = jsonMap[k]
		}
	
	}
	if (jsonMap["host"].(string) != "" && len(resourceMap)==0){
		resourceMap["system.hostname"] = jsonMap["host"]
	}

	if(logicmonitorClient.includeMetadata){
		metadata = getMetadata(jsonMap)
	}

  //Check if message or log is present in the log else return error
	message, ok := jsonMap["message"].(string)
  if !ok {
        logger.Log("Cannot convert message to string")
        message, ok = jsonMap["log"].(string)
        if !ok {
            logger.Log("No message or log field in the record")
        } else {
            logger.Debug(fmt.Sprintf("Log field found: %s", message))
        }
  } else {
        logger.Debug(fmt.Sprintf("Message field found: %s", message))
  }

  //Send logs only if message is not empty
  if message != "" {
      logs := model.LogInput{
      Message:    message,
      Timestamp: jsonMap["timestamp"].(string),
      ResourceID: resourceMap,
      Metadata: metadata,
    }
    logger.Debug(fmt.Sprintf("adding log to the bulk: %+v\n", logs))
    logicmonitorClient.bulk = append(logicmonitorClient.bulk, logs)
  }
	return output.FLB_OK
	
}

func getMetadata(log map[string]interface{}) map[string]interface{}{

	metadata := make(map[string]interface{})
	for key, value := range log{
		if(key != "message" && key != "log" && key != "timestamp"){
			metadata[key]= value
		}
	}

	return metadata

}

func (logicmonitorClient *LogicmonitorClient) sendBulk(logIngestor *logs.LMLogIngest) int {

	if len(logicmonitorClient.bulk) == 0 {
		return output.FLB_OK
	}

	fmt.Println("Sending bulk of length ", len(logicmonitorClient.bulk))
	ingestResponse, err := logIngestor.SendLogs(context.Background(), logicmonitorClient.bulk)
	if(err != nil){
		logger.Log(fmt.Sprintf("Error in sending logs to LM : %s",err))
	}
	logger.Log(fmt.Sprintf("Response received from LM Log Ingest : %s",ingestResponse))
	return output.FLB_OK

}

// Flush sends one last bulk
func (logicmonitorClient *LogicmonitorClient) Flush(logIngestor *logs.LMLogIngest) int {
	resp := logicmonitorClient.sendBulk(logIngestor)
	logicmonitorClient.bulk = nil
	return resp
}

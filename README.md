
# lm-logs-fluentbit
This output plugin sends Fluentbit records to the configured LogicMonitor account.

## Prerequisites

Install the plugin:
* Install the Fluentbit plugin:       `curl https://raw.githubusercontent.com/fluent/fluent-bit/master/install.sh | sh`

## Configure the output plugin

Create a custom `fluent-bit.conf` or edit the existing one to specify which logs should be forwarded to LogicMonitor.

```
# Match events tagged with "lm.**" and
# send them to LogicMonitor
[SERVICE]
    Flush        5

[INPUT]
    Name        <name>
    Path        <filename>

[OUTPUT]
    Name <name>
    lmCompanyName  <company_name_with_domain>
    Match *
    Workers 1
    accessKey <access_key>
    accessID <access_ID>
    bearerToken Bearer <bearer_token>
    resourceMapping {"<event_key>": "<lm_property>"}
    includeMetadata <boolean_value>
    lmDebug <boolean_value>
```

For more configuration examples, please refer to the examples folder, or see the [Fluentbit configuration documentation](https://docs.fluentbit.io/manual/administration/configuring-fluent-bit/classic-mode/configuration-file)

### Request example

Sending:

`curl -X POST -d 'json={"message":"hello LogicMonitor from fluentbit", "event_key":"lm_property_value"}' http://localhost:8888/lm.test`

Produces this event:
```
{
    "message": "hello LogicMonitor from fluentbit"
}
```

**Note:** Make sure that logs have a message field. Requests sent without a message will not be accepted. 


### Resource mapping examples

- `{"message":"Hey!!", "event_key":"lm_property_value"}` with mapping `{"event_key": "lm_property"}`
- `{"message":"Hey!!", "_lm.resourceId": { "lm_property_name" : "lm_property_value" } }`  this will override resource mapping.

## LogicMonitor properties

| Property          | Description                                                                                                                                                                                            |
|-------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `Name`            | Name of the input plugin.                                                                                                                                                                              |
| `lmCompanyName`   | LogicMonitor account name with domain. For example, test.logicmonitor.com .                                                                                                                            |
| `Match`           | A pattern to match against the tags of incoming records. For example, * will match everything.                                                                                                         |
| `Workers`         | Number of workers to operate.                                                                                                                                                                          |
| `accessID`        | LM API Token access ID. If not provided, omit setting the key entirely.                                                                                                                                |
| `accessKey`       | LM API Token access key. If not provided, omit setting the key entirely.                                                                                                                               |
| `bearerToken`     | LM API Bearer Token. Either specify `access_id` and `access_key` both or `bearer_token`. If all specified, LMv1 token(`access_id` and `access_key`) will be used for authentication with Logicmonitor. |
| `resourceMapping` | The mapping that defines the source of the log event to the LM resource. In this case, the `<event_key>` in the incoming event is mapped to the value of `<lm_property>`.                              |
| `includeMetadata` | When `true`, appends additional metadata to the log. default `false`.                                                                                                                                  |
| `lmDebug`         | When `true`, logs more information to the fluent-bit console.                                                                                                                                          |



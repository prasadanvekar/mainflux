package influxdb

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/mainflux/mainflux/readers"

	influxdata "github.com/influxdata/influxdb/client/v2"
	"github.com/mainflux/mainflux"
)

const maxLimit = 100

var _ readers.MessageRepository = (*influxRepository)(nil)

type influxRepository struct {
	database string
	client   influxdata.Client
}

type fields map[string]interface{}
type tags map[string]string

// New returns new InfluxDB reader.
func New(client influxdata.Client, database string) (readers.MessageRepository, error) {
	return &influxRepository{database, client}, nil
}

func (repo *influxRepository) ReadAll(chanID, offset, limit uint64) []mainflux.Message {
	if limit > maxLimit {
		limit = maxLimit
	}

	cmd := fmt.Sprintf(`SELECT * from messages WHERE Channel='%d' LIMIT %d OFFSET %d`, chanID, limit, offset)
	q := influxdata.Query{
		Command:  cmd,
		Database: repo.database,
	}

	ret := []mainflux.Message{}

	resp, err := repo.client.Query(q)
	if err != nil || resp.Error() != nil {
		return ret
	}

	if len(resp.Results) < 1 || len(resp.Results[0].Series) < 1 {
		return ret
	}
	result := resp.Results[0].Series[0]
	for _, v := range result.Values {
		ret = append(ret, parseMessage(result.Columns, v))
	}

	return ret
}

// ParseMessage and parseValues are util methods. Since InfluxDB client returns
// results in form of rows and columns, this obscure message conversion is needed
// to return actual []mainflux.Message from the query result.
func parseValues(value interface{}, name string, msg *mainflux.Message) {
	if name == "ValueSum" && value != nil {
		if sum, ok := value.(json.Number); ok {
			valSum, err := sum.Float64()
			if err != nil {
				return
			}
			msg.ValueSum = &mainflux.SumValue{Value: valSum}
		}
		return
	}
	if strings.HasSuffix(name, "Value") {
		switch value.(type) {
		case bool:
			msg.Value = &mainflux.Message_BoolValue{value.(bool)}
		case json.Number:
			num, err := value.(json.Number).Float64()
			if err != nil {
				return
			}

			msg.Value = &mainflux.Message_FloatValue{num}
		case string:
			if strings.HasPrefix(name, "String") {
				msg.Value = &mainflux.Message_StringValue{value.(string)}
				return
			}

			if strings.HasPrefix(name, "Data") {
				msg.Value = &mainflux.Message_DataValue{value.(string)}
			}
		}
	}
}

func parseMessage(names []string, fields []interface{}) mainflux.Message {
	m := mainflux.Message{}
	v := reflect.ValueOf(&m).Elem()
	for i, name := range names {
		parseValues(fields[i], name, &m)
		msgField := v.FieldByName(name)
		if !msgField.IsValid() {
			continue
		}

		f := msgField.Interface()
		switch f.(type) {
		case string:
			if s, ok := fields[i].(string); ok {
				msgField.SetString(s)
			}
		case uint64:
			u, _ := strconv.ParseUint(fields[i].(string), 10, 64)
			msgField.SetUint(u)
		case float64:
			val, _ := strconv.ParseFloat(fields[i].(string), 64)
			msgField.SetFloat(val)
		}
	}

	return m
}

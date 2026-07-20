package schema

import (
	"reflect"
	"strings"
)

// DataType represents a PostgreSQL data type.
type DataType uint8

const (
	BigInt DataType = iota
	Boolean
	Bytea
	Char
	CIDText
	Date
	Decimal
	Double
	Real
	Float
	Inet
	Integer
	Interval
	Json
	JsonB
	Money
	Numeric
	Oid
	OID
	Path
	Point
	Polygon
	SmallInt
	Text
	Time
	TimeWithTimeZone
	Timestamp
	TimestampWithTimeZone
	TimestampTZ
	Varchar
	XML
	UUID
	TSVector
	TSQuery
	Bit
	VarBit
	MacAddr
	CIDR
	Serial
	BigSerial
	SmallSerial
	HStore
	CIText
	Int4Range
	Int4Multirange
	Int8Range
	Int8Multirange
	NumRange
	NumMultirange
	TsRange
	TsMultirange
	TsZRange
	TsZMultirange
	DateRange
	DateMultirange
	XmlPath
	Line
	Box
	Circle
	LineSegment
)

var dataTypeText = map[DataType][]string{
	BigInt:                {"bigint", "int8"},
	Boolean:               {"boolean", "bool"},
	Bytea:                 {"bytea"},
	Char:                  {"char", "character"},
	CIDText:               {"cidr"},
	Date:                  {"date"},
	Decimal:               {"decimal", "numeric"},
	Double:                {"double precision", "float8"},
	Real:                  {"real", "float4"},
	Float:                 {"float"},
	Inet:                  {"inet"},
	Integer:               {"integer", "int", "int4"},
	Interval:              {"interval"},
	Json:                  {"json"},
	JsonB:                 {"jsonb"},
	Money:                 {"money"},
	Numeric:               {"numeric"},
	Oid:                   {"oid"},
	Path:                  {"path"},
	Point:                 {"point"},
	Polygon:               {"polygon"},
	SmallInt:              {"smallint", "int2"},
	Text:                  {"text"},
	Time:                  {"time"},
	TimeWithTimeZone:      {"time with time zone", "timetz"},
	Timestamp:             {"timestamp without time zone", "timestamp"},
	TimestampWithTimeZone: {"timestamp with time zone", "timestamptz"},
	TimestampTZ:           {"timestamptz"},
	Varchar:               {"varchar", "character varying", "varchar(255)", "nvarchar"},
	XML:                   {"xml"},
	UUID:                  {"uuid"},
	TSVector:              {"tsvector"},
	TSQuery:               {"tsquery"},
	Bit:                   {"bit"},
	VarBit:                {"bit varying", "varbit"},
	MacAddr:               {"macaddr", "macaddr8"},
	CIDR:                  {"cidr"},
	Serial:                {"serial"},
	BigSerial:             {"bigserial"},
	SmallSerial:           {"smallserial"},
	HStore:                {"hstore"},
	CIText:                {"citext"},
	Int4Range:             {"int4range"},
	Int4Multirange:        {"int4multirange"},
	Int8Range:             {"int8range"},
	Int8Multirange:        {"int8multirange"},
	NumRange:              {"numrange"},
	NumMultirange:         {"nummultirange"},
	TsRange:               {"tsrange"},
	TsMultirange:          {"tsmultirange"},
	TsZRange:              {"tstzrange"},
	TsZMultirange:         {"tstzmultirange"},
	DateRange:             {"daterange"},
	DateMultirange:        {"datemultirange"},
	XmlPath:               {"xmlpath"},
	Line:                  {"line"},
	Box:                   {"box"},
	Circle:                {"circle"},
	LineSegment:           {"lseg"},
}

// RegisterDataType registers a custom data type with its SQL names.
func RegisterDataType(t DataType, names ...string) {
	dataTypeText[t] = names
}

func (t DataType) String() string {
	if names, ok := dataTypeText[t]; ok && len(names) > 0 {
		return names[0]
	}
	return "text"
}

// IsString returns true if the given SQL type string matches this DataType.
func (t DataType) IsString(s string) bool {
	names, ok := dataTypeText[t]
	if !ok {
		return false
	}
	lower := strings.ToLower(s)
	for _, name := range names {
		if strings.HasPrefix(lower, name) {
			return true
		}
	}
	return false
}

// IsArray returns true if this data type is an array type.
func (t DataType) IsArray() bool {
	return false // arrays are handled via type argument
}

// IsTime returns true if this data type represents time/date/timestamp.
func (t DataType) IsTime() bool {
	switch t {
	case Date, Time, TimeWithTimeZone, Timestamp, TimestampWithTimeZone, TimestampTZ:
		return true
	}
	return false
}

// GoType returns the Go reflect.Type for this DataType.
func (t DataType) GoType() reflect.Type {
	return dataTypeToGoType(t)
}

// ParseDataType parses a SQL data type string into a DataType and optional type argument.
// For example, "varchar(255)" returns (Varchar, "255").
func ParseDataType(s string) (DataType, string) {
	lower := strings.ToLower(s)
	// Check for parenthesized argument
	argStart := strings.Index(lower, "(")
	if argStart != -1 {
		argEnd := strings.Index(lower[argStart:], ")")
		if argEnd != -1 {
			typeName := lower[:argStart]
			arg := s[argStart+1 : argStart+argEnd]
			for dt, names := range dataTypeText {
				for _, name := range names {
					if name == typeName {
						return dt, arg
					}
				}
			}
		}
	}

	// Check without argument
	for dt, names := range dataTypeText {
		for _, name := range names {
			if name == lower {
				return dt, ""
			}
		}
	}

	return Text, ""
}

func dataTypeToGoType(dataType DataType) reflect.Type {
	switch dataType {
	case BigInt, BigSerial:
		return reflect.TypeOf(int64(0))
	case Boolean:
		return reflect.TypeOf(false)
	case Bytea:
		return reflect.TypeOf([]byte{})
	case Integer, Serial, SmallInt, SmallSerial:
		return reflect.TypeOf(int(0))
	case Real, Float:
		return reflect.TypeOf(float32(0))
	case Double, Decimal, Numeric, Money:
		return reflect.TypeOf(float64(0))
	case Date, Time, TimeWithTimeZone, Timestamp, TimestampWithTimeZone, TimestampTZ:
		return reflect.TypeOf("")
	case Varchar, Char, Text, CIDText:
		return reflect.TypeOf("")
	case Json, JsonB:
		return reflect.TypeOf("")
	case UUID:
		return reflect.TypeOf("")
	case Boolean + 100: // placeholder, handled above
		return reflect.TypeOf(false)
	default:
		return reflect.TypeOf("")
	}
}

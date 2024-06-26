// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        v5.27.1
// source: dropsonde-protocol/events/log.proto

package events

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// MessageType stores the destination of the message (corresponding to STDOUT or STDERR).
type LogMessage_MessageType int32

const (
	LogMessage_OUT LogMessage_MessageType = 1
	LogMessage_ERR LogMessage_MessageType = 2
)

// Enum value maps for LogMessage_MessageType.
var (
	LogMessage_MessageType_name = map[int32]string{
		1: "OUT",
		2: "ERR",
	}
	LogMessage_MessageType_value = map[string]int32{
		"OUT": 1,
		"ERR": 2,
	}
)

func (x LogMessage_MessageType) Enum() *LogMessage_MessageType {
	p := new(LogMessage_MessageType)
	*p = x
	return p
}

func (x LogMessage_MessageType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (LogMessage_MessageType) Descriptor() protoreflect.EnumDescriptor {
	return file_dropsonde_protocol_events_log_proto_enumTypes[0].Descriptor()
}

func (LogMessage_MessageType) Type() protoreflect.EnumType {
	return &file_dropsonde_protocol_events_log_proto_enumTypes[0]
}

func (x LogMessage_MessageType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Do not use.
func (x *LogMessage_MessageType) UnmarshalJSON(b []byte) error {
	num, err := protoimpl.X.UnmarshalJSONEnum(x.Descriptor(), b)
	if err != nil {
		return err
	}
	*x = LogMessage_MessageType(num)
	return nil
}

// Deprecated: Use LogMessage_MessageType.Descriptor instead.
func (LogMessage_MessageType) EnumDescriptor() ([]byte, []int) {
	return file_dropsonde_protocol_events_log_proto_rawDescGZIP(), []int{0, 0}
}

// A LogMessage contains a "log line" and associated metadata.
type LogMessage struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Bytes of the log message. (Note that it is not required to be a single line.)
	Message []byte `protobuf:"bytes,1,req,name=message" json:"message,omitempty"`
	// Type of the message (OUT or ERR).
	MessageType *LogMessage_MessageType `protobuf:"varint,2,req,name=message_type,json=messageType,enum=events.LogMessage_MessageType" json:"message_type,omitempty"`
	// UNIX timestamp (in nanoseconds) when the log was written.
	Timestamp *int64 `protobuf:"varint,3,req,name=timestamp" json:"timestamp,omitempty"`
	// Application that emitted the message (or to which the application is related).
	AppId *string `protobuf:"bytes,4,opt,name=app_id,json=appId" json:"app_id,omitempty"`
	// Source of the message. For Cloud Foundry, this can be "APP", "RTR", "DEA", "STG", etc.
	SourceType *string `protobuf:"bytes,5,opt,name=source_type,json=sourceType" json:"source_type,omitempty"`
	// Instance that emitted the message.
	SourceInstance *string `protobuf:"bytes,6,opt,name=source_instance,json=sourceInstance" json:"source_instance,omitempty"`
}

func (x *LogMessage) Reset() {
	*x = LogMessage{}
	if protoimpl.UnsafeEnabled {
		mi := &file_dropsonde_protocol_events_log_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *LogMessage) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*LogMessage) ProtoMessage() {}

func (x *LogMessage) ProtoReflect() protoreflect.Message {
	mi := &file_dropsonde_protocol_events_log_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use LogMessage.ProtoReflect.Descriptor instead.
func (*LogMessage) Descriptor() ([]byte, []int) {
	return file_dropsonde_protocol_events_log_proto_rawDescGZIP(), []int{0}
}

func (x *LogMessage) GetMessage() []byte {
	if x != nil {
		return x.Message
	}
	return nil
}

func (x *LogMessage) GetMessageType() LogMessage_MessageType {
	if x != nil && x.MessageType != nil {
		return *x.MessageType
	}
	return LogMessage_OUT
}

func (x *LogMessage) GetTimestamp() int64 {
	if x != nil && x.Timestamp != nil {
		return *x.Timestamp
	}
	return 0
}

func (x *LogMessage) GetAppId() string {
	if x != nil && x.AppId != nil {
		return *x.AppId
	}
	return ""
}

func (x *LogMessage) GetSourceType() string {
	if x != nil && x.SourceType != nil {
		return *x.SourceType
	}
	return ""
}

func (x *LogMessage) GetSourceInstance() string {
	if x != nil && x.SourceInstance != nil {
		return *x.SourceInstance
	}
	return ""
}

var File_dropsonde_protocol_events_log_proto protoreflect.FileDescriptor

var file_dropsonde_protocol_events_log_proto_rawDesc = []byte{
	0x0a, 0x23, 0x64, 0x72, 0x6f, 0x70, 0x73, 0x6f, 0x6e, 0x64, 0x65, 0x2d, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x63, 0x6f, 0x6c, 0x2f, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x73, 0x2f, 0x6c, 0x6f, 0x67, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x73, 0x22, 0x89, 0x02,
	0x0a, 0x0a, 0x4c, 0x6f, 0x67, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x18, 0x0a, 0x07,
	0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x02, 0x28, 0x0c, 0x52, 0x07, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x12, 0x41, 0x0a, 0x0c, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x02, 0x20, 0x02, 0x28, 0x0e, 0x32, 0x1e, 0x2e, 0x65,
	0x76, 0x65, 0x6e, 0x74, 0x73, 0x2e, 0x4c, 0x6f, 0x67, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x2e, 0x4d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x54, 0x79, 0x70, 0x65, 0x52, 0x0b, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x74, 0x69, 0x6d,
	0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x18, 0x03, 0x20, 0x02, 0x28, 0x03, 0x52, 0x09, 0x74, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x12, 0x15, 0x0a, 0x06, 0x61, 0x70, 0x70, 0x5f, 0x69,
	0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x61, 0x70, 0x70, 0x49, 0x64, 0x12, 0x1f,
	0x0a, 0x0b, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12,
	0x27, 0x0a, 0x0f, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x6e, 0x73, 0x74, 0x61, 0x6e,
	0x63, 0x65, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65,
	0x49, 0x6e, 0x73, 0x74, 0x61, 0x6e, 0x63, 0x65, 0x22, 0x1f, 0x0a, 0x0b, 0x4d, 0x65, 0x73, 0x73,
	0x61, 0x67, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x07, 0x0a, 0x03, 0x4f, 0x55, 0x54, 0x10, 0x01,
	0x12, 0x07, 0x0a, 0x03, 0x45, 0x52, 0x52, 0x10, 0x02, 0x42, 0x58, 0x0a, 0x21, 0x6f, 0x72, 0x67,
	0x2e, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x66, 0x6f, 0x75, 0x6e, 0x64, 0x72, 0x79, 0x2e, 0x64, 0x72,
	0x6f, 0x70, 0x73, 0x6f, 0x6e, 0x64, 0x65, 0x2e, 0x65, 0x76, 0x65, 0x6e, 0x74, 0x73, 0x42, 0x0a,
	0x4c, 0x6f, 0x67, 0x46, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x79, 0x5a, 0x27, 0x67, 0x69, 0x74, 0x68,
	0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x63, 0x6c, 0x6f, 0x75, 0x64, 0x66, 0x6f, 0x75, 0x6e,
	0x64, 0x72, 0x79, 0x2f, 0x73, 0x6f, 0x6e, 0x64, 0x65, 0x2d, 0x67, 0x6f, 0x2f, 0x65, 0x76, 0x65,
	0x6e, 0x74, 0x73,
}

var (
	file_dropsonde_protocol_events_log_proto_rawDescOnce sync.Once
	file_dropsonde_protocol_events_log_proto_rawDescData = file_dropsonde_protocol_events_log_proto_rawDesc
)

func file_dropsonde_protocol_events_log_proto_rawDescGZIP() []byte {
	file_dropsonde_protocol_events_log_proto_rawDescOnce.Do(func() {
		file_dropsonde_protocol_events_log_proto_rawDescData = protoimpl.X.CompressGZIP(file_dropsonde_protocol_events_log_proto_rawDescData)
	})
	return file_dropsonde_protocol_events_log_proto_rawDescData
}

var file_dropsonde_protocol_events_log_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_dropsonde_protocol_events_log_proto_msgTypes = make([]protoimpl.MessageInfo, 1)
var file_dropsonde_protocol_events_log_proto_goTypes = []any{
	(LogMessage_MessageType)(0), // 0: events.LogMessage.MessageType
	(*LogMessage)(nil),          // 1: events.LogMessage
}
var file_dropsonde_protocol_events_log_proto_depIdxs = []int32{
	0, // 0: events.LogMessage.message_type:type_name -> events.LogMessage.MessageType
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_dropsonde_protocol_events_log_proto_init() }
func file_dropsonde_protocol_events_log_proto_init() {
	if File_dropsonde_protocol_events_log_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_dropsonde_protocol_events_log_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*LogMessage); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_dropsonde_protocol_events_log_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   1,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_dropsonde_protocol_events_log_proto_goTypes,
		DependencyIndexes: file_dropsonde_protocol_events_log_proto_depIdxs,
		EnumInfos:         file_dropsonde_protocol_events_log_proto_enumTypes,
		MessageInfos:      file_dropsonde_protocol_events_log_proto_msgTypes,
	}.Build()
	File_dropsonde_protocol_events_log_proto = out.File
	file_dropsonde_protocol_events_log_proto_rawDesc = nil
	file_dropsonde_protocol_events_log_proto_goTypes = nil
	file_dropsonde_protocol_events_log_proto_depIdxs = nil
}

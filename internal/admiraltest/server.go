// Package admiraltest is the Admiral API as the CLI sees it, for tests. It
// answers any RPC on any Admiral service with a response built from the
// proto descriptors, so it needs no fixture: a new field, oneof branch or
// enum value is served the day the SDK has it.
//
// A Shape says how the response is filled: sparse (the response's own
// fields set, every message under them empty, as a resource the server
// filled only partly), every field set once per oneof branch with enums and
// booleans cycling, or every list empty. A response with nothing in it at
// all breaks the API's contract, so no shape sends one.
package admiraltest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Shape is one way of filling a response.
type Shape struct {
	sparse     bool // only the response's own fields set, every message under them empty
	branch     int  // which oneof branch, enum value and boolean to pick
	emptyLists bool // every repeated and map field left empty
}

func (s Shape) String() string {
	switch {
	case s.sparse:
		return "sparse"
	case s.emptyLists:
		return fmt.Sprintf("branch-%d-empty-lists", s.branch)
	}
	return fmt.Sprintf("branch-%d", s.branch)
}

// Shapes is every shape worth sending: sparse, each branch, and one with
// empty lists. The branch count covers the widest oneof and the enums
// commands switch on.
func Shapes() []Shape {
	out := []Shape{{sparse: true, emptyLists: true}}
	for b := range 8 {
		out = append(out, Shape{branch: b})
	}
	return append(out, Shape{branch: 1, emptyLists: true})
}

// Server answers any RPC on any Admiral service with its current shape.
type Server struct {
	addr    string
	current atomic.Pointer[Shape]
}

// Start serves on a loopback port until the test ends, answering sparse
// until Respond says otherwise.
func Start(t testing.TB) *Server {
	t.Helper()
	s := &Server{}
	s.current.Store(&Shape{sparse: true, emptyLists: true})
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("admiraltest: listen: %v", err)
	}
	srv := grpc.NewServer(grpc.UnknownServiceHandler(s.handle))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	s.addr = lis.Addr().String()
	return s
}

// Addr is the host:port to pass as --server, with --plaintext.
func (s *Server) Addr() string { return s.addr }

// Respond makes every later call answer with shape.
func (s *Server) Respond(shape Shape) { s.current.Store(&shape) }

func (s *Server) handle(_ any, stream grpc.ServerStream) error {
	full, ok := grpc.MethodFromServerStream(stream)
	if !ok {
		return errors.New("no method")
	}
	svc, method, _ := strings.Cut(strings.TrimPrefix(full, "/"), "/")
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(svc))
	if err != nil {
		return err
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return fmt.Errorf("%s is not a service", svc)
	}
	md := sd.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return fmt.Errorf("no method %s", full)
	}
	for {
		if err := stream.RecvMsg(dynamicpb.NewMessage(md.Input())); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if !md.IsStreamingClient() {
			break
		}
	}
	mt, err := protoregistry.GlobalTypes.FindMessageByName(md.Output().FullName())
	if err != nil {
		return err
	}
	out := mt.New()
	fill(out, *s.current.Load(), 0)
	return stream.SendMsg(out.Interface())
}

// fill sets m's fields for s. Recursion stops at a depth that covers every
// nesting a command reads without walking self-referencing messages forever.
func fill(m protoreflect.Message, s Shape, depth int) {
	if depth > 6 || (s.sparse && depth > 0) {
		return
	}
	if wellKnown(m, s) {
		return
	}
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		// A oneof takes each branch in turn, and every n+1th shape none, which
		// is how an infra agent's assignment arrives without Kubernetes settings.
		if o := f.ContainingOneof(); o != nil && !o.IsSynthetic() {
			if pick := s.branch % (o.Fields().Len() + 1); pick == o.Fields().Len() || o.Fields().Get(pick) != f {
				continue
			}
		}
		switch {
		case f.IsList():
			if s.emptyLists {
				continue
			}
			l := m.NewField(f).List()
			for i := range 2 {
				if f.Message() != nil {
					e := l.NewElement()
					fill(e.Message(), s, depth+1)
					if i == 1 {
						rename(e.Message())
					}
					l.Append(e)
					continue
				}
				l.Append(scalar(f, s))
			}
			m.Set(f, protoreflect.ValueOfList(l))
		case f.IsMap():
			if s.emptyLists {
				continue
			}
			mp := m.NewField(f).Map()
			v := scalar(f.MapValue(), s)
			if f.MapValue().Message() != nil {
				v = mp.NewValue()
				fill(v.Message(), s, depth+1)
			}
			mp.Set(scalar(f.MapKey(), s).MapKey(), v)
			m.Set(f, protoreflect.ValueOfMap(mp))
		case f.Message() != nil:
			v := m.NewField(f)
			fill(v.Message(), s, depth+1)
			m.Set(f, v)
		default:
			m.Set(f, scalar(f, s))
		}
	}
}

// rename gives a list's second element its own name, so a command that
// looks a name up finds exactly one match, as it would on a real server.
func rename(m protoreflect.Message) {
	if f := m.Descriptor().Fields().ByName("name"); f != nil && f.Kind() == protoreflect.StringKind && !f.IsList() {
		m.Set(f, protoreflect.ValueOfString("y"))
	}
}

// wellKnown fills the types whose meaning is in their value, so a
// timestamp is a time and a struct holds a value.
func wellKnown(m protoreflect.Message, s Shape) bool {
	var v proto.Message
	switch m.Descriptor().FullName() {
	case "google.protobuf.Timestamp":
		v = timestamppb.New(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	case "google.protobuf.Duration":
		v = durationpb.New(time.Minute)
	case "google.protobuf.FieldMask":
		v = &fieldmaskpb.FieldMask{Paths: []string{"name"}}
	case "google.protobuf.Struct":
		v, _ = structpb.NewStruct(map[string]any{"a": map[string]any{"b": []any{"c", float64(s.branch)}}})
	case "google.protobuf.Value":
		v = []*structpb.Value{
			structpb.NewStringValue("x"), structpb.NewNullValue(), structpb.NewNumberValue(1),
			structpb.NewBoolValue(true), structpb.NewListValue(&structpb.ListValue{}),
		}[s.branch%5]
	case "google.protobuf.ListValue":
		v, _ = structpb.NewList([]any{"x", 1.0})
	case "google.protobuf.Any":
		return true
	default:
		return false
	}
	src := v.ProtoReflect()
	src.Range(func(f protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		m.Set(m.Descriptor().Fields().ByNumber(f.Number()), val)
		return true
	})
	return true
}

func scalar(f protoreflect.FieldDescriptor, s Shape) protoreflect.Value {
	switch f.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(stringFor(f))
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(s.branch%2 == 1)
	case protoreflect.EnumKind:
		vals := f.Enum().Values()
		return protoreflect.ValueOfEnum(vals.Get(s.branch % vals.Len()).Number())
	case protoreflect.BytesKind:
		return protoreflect.ValueOfBytes([]byte("x"))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(1.5)
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(1.5)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(s.branch + 1))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(s.branch + 1))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(s.branch + 1))
	default:
		return protoreflect.ValueOfInt64(int64(s.branch + 1))
	}
}

// idPrefixes are the typed id prefixes, keyed by the field or message they
// name, so a command that checks an id's kind accepts the response's.
var idPrefixes = map[string]string{
	"application": "app", "environment": "env", "agent": "agt", "change_set": "cs", "changeset": "cs",
	"run": "run", "revision": "rev", "job": "job", "service_account": "sa", "user": "usr",
	"tenant": "tnt", "assignment": "asg", "group": "grp", "issuer": "iss", "credential": "cred",
	"template": "cmp", "component": "cmp", "version": "ver", "api_key": "key", "key": "key",
}

func stringFor(f protoreflect.FieldDescriptor) string {
	name := string(f.Name())
	switch {
	case name == "next_page_token":
		// A real list ends; a token on every page would page forever.
		return ""
	case name == "id":
		return typedID(snake(string(f.ContainingMessage().Name())))
	case strings.HasSuffix(name, "_id"):
		return typedID(strings.TrimSuffix(name, "_id"))
	case strings.HasSuffix(name, "_json"):
		return `{"a":{"b":[1,"x",null]},"c":"d"}`
	case name == "digest" || strings.HasSuffix(name, "_digest"):
		return "sha256:" + strings.Repeat("ab", 32)
	case strings.Contains(name, "email"):
		return "x@example.com"
	case strings.Contains(name, "url") || strings.Contains(name, "uri"):
		return "https://example.com/x"
	case strings.Contains(name, "version"):
		return "1.2.3"
	}
	return "x"
}

func typedID(kind string) string {
	if p, ok := idPrefixes[kind]; ok {
		return p + "-000000000001"
	}
	return "x-000000000001"
}

func snake(camel string) string {
	var b strings.Builder
	for i, r := range camel {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

package sarama

import (
	"context"
	"errors"
	"testing"
	"time"

	native "github.com/IBM/sarama"
)

type transactionalFixture struct {
	*asyncFixture
	called   chan string
	failure  error
	offsets  map[string][]*native.PartitionOffsetMetadata
	groupID  string
	group    *native.ConsumerGroupMetadata
	message  *native.ConsumerMessage
	metadata *string
}

func (f *transactionalFixture) BeginTxn() error  { f.called <- "begin"; return f.failure }
func (f *transactionalFixture) CommitTxn() error { f.called <- "commit"; return f.failure }
func (f *transactionalFixture) AbortTxn() error  { f.called <- "abort"; return f.failure }
func (f *transactionalFixture) AddOffsetsToTxn(offsets map[string][]*native.PartitionOffsetMetadata, group string) error {
	f.offsets, f.groupID = offsets, group
	return f.failure
}
func (f *transactionalFixture) AddOffsetsToTxnWithGroupMetadata(offsets map[string][]*native.PartitionOffsetMetadata, group *native.ConsumerGroupMetadata) error {
	f.offsets, f.group = offsets, group
	return f.failure
}
func (f *transactionalFixture) AddMessageToTxn(message *native.ConsumerMessage, group string, metadata *string) error {
	f.message, f.groupID, f.metadata = message, group, metadata
	return f.failure
}
func (f *transactionalFixture) AddMessageToTxnWithGroupMetadata(message *native.ConsumerMessage, group *native.ConsumerGroupMetadata, metadata *string) error {
	f.message, f.group, f.metadata = message, group, metadata
	return f.failure
}

func TestAsyncTransactionCannotOvertakeAcceptedMessage(t *testing.T) {
	for _, useSend := range []bool{false, true} {
		f := &transactionalFixture{asyncFixture: newAsyncFixture(), called: make(chan string, 1), failure: errors.New("native transaction error")}
		w, err := WrapAsyncProducer(f, asyncConfig(), nil)
		if err != nil {
			t.Fatal(err)
		}
		original := &native.ProducerMessage{Topic: "orders"}
		if useSend {
			if err = w.Send(context.Background(), original); err != nil {
				t.Fatal(err)
			}
		} else {
			w.Input() <- original
		}
		done := make(chan error, 1)
		go func() { done <- w.CommitTxn() }()
		select {
		case <-f.called:
			t.Fatal("transaction overtook accepted message")
		case <-time.After(20 * time.Millisecond):
		}
		sent := receiveMessage(t, f.input)
		if err = <-done; err != f.failure {
			t.Fatal("transaction error identity", err)
		}
		if <-f.called != "commit" {
			t.Fatal("wrong operation")
		}
		f.successes <- sent
		if receiveMessage(t, w.Successes()) != original {
			t.Fatal("completion identity")
		}
		for name, op := range map[string]func() error{"begin": w.BeginTxn, "abort": w.AbortTxn} {
			if op() != f.failure || <-f.called != name {
				t.Fatal("transaction delegation", name)
			}
		}
		offsets := map[string][]*native.PartitionOffsetMetadata{"orders": {{Partition: 2, Offset: 42}}}
		group := &native.ConsumerGroupMetadata{}
		message := &native.ConsumerMessage{Topic: "orders", Offset: 42}
		metadata := "application"
		if w.AddOffsetsToTxn(offsets, "group") != f.failure || f.groupID != "group" || f.offsets["orders"][0] != offsets["orders"][0] {
			t.Fatal("offset delegation")
		}
		if w.AddOffsetsToTxnWithGroupMetadata(offsets, group) != f.failure || f.group != group {
			t.Fatal("group fencing metadata")
		}
		if w.AddMessageToTxn(message, "group", &metadata) != f.failure || f.message != message || f.metadata != &metadata {
			t.Fatal("message delegation")
		}
		if w.AddMessageToTxnWithGroupMetadata(message, group, &metadata) != f.failure || f.group != group {
			t.Fatal("message group metadata")
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		if w.CommitTxn() != native.ErrShuttingDown {
			t.Fatal("transaction after close")
		}
	}
}

func TestAsyncClosedInputShutsDown(t *testing.T) {
	f := newAsyncFixture()
	w, err := WrapAsyncProducer(f, asyncConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	close(w.Input())
	select {
	case <-f.closed:
	case <-time.After(time.Second):
		t.Fatal("closed input did not shut down")
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
}

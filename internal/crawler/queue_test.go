package crawler

import (
	"testing"
)

func TestTaskQueue_OrderAndReset(t *testing.T) {
	q := &taskQueue{}

	if q.Len() != 0 {
		t.Fatalf("expected empty queue, got %d", q.Len())
	}

	_, ok := q.Pop()
	if ok {
		t.Fatal("expected pop from empty queue to return false")
	}

	task1 := CrawlTask{URL: "https://example.com/1", Level: 1}
	task2 := CrawlTask{URL: "https://example.com/2", Level: 2}

	q.Push(task1)
	q.Push(task2)

	if q.Len() != 2 {
		t.Fatalf("expected queue length 2, got %d", q.Len())
	}

	peeked, ok := q.Peek()
	if !ok || peeked.URL != task1.URL {
		t.Fatalf("expected peek to return task1, got %v", peeked)
	}

	if q.Len() != 2 {
		t.Fatal("peek should not remove element from queue")
	}

	popped1, ok := q.Pop()
	if !ok || popped1.URL != task1.URL {
		t.Fatalf("expected pop to return task1, got %v", popped1)
	}

	popped2, ok := q.Pop()
	if !ok || popped2.URL != task2.URL {
		t.Fatalf("expected pop to return task2, got %v", popped2)
	}

	if q.Len() != 0 {
		t.Fatalf("expected queue to be empty after popping all, got %d", q.Len())
	}

	if q.head != 0 {
		t.Fatalf("expected queue head to reset to 0, got %d", q.head)
	}
}

package fqueue

import (
	"os"
	"unsafe"
)

/*
#include <fcntl.h>
#include <unistd.h>
#include <sys/mman.h>
*/
import "C"

type Writer struct {
	p      []byte
	offset int64
	ptr    unsafe.Pointer
	fd     *os.File
	*FQueue
}

func (b *Writer) rolling() error {
	b.unmapper()
	b.WriterOffset = MetaSize
	b.offset = int64(b.WriterOffset)
	b.p = b.p[:0]
	return nil
}

func (b *Writer) setBottom() {
	if b.ReaderOffset < b.WriterOffset && b.WriterOffset > b.WriterBottom && b.FSize < b.Limit {
		b.WriterBottom = b.WriterOffset
	}
}

func (b *Writer) Close() error {
	if err := b.unmapper(); err != nil {
		return err
	}
	return b.fd.Close()
}

func (b *Writer) unmapper() error {
	if b.ptr != nil {
		if c := C.munmap(b.ptr, PageSize); c != 0 {
			return MunMapErr
		}
		b.ptr = nil
	}
	return nil
}

func (b *Writer) mapper() (err error) {
	if err = b.unmapper(); err != nil {
		return
	}
	b.ptr, err = C.mmap(nil, C.size_t(PageSize), C.PROT_WRITE, C.MAP_SHARED, C.int(b.fd.Fd()), C.off_t(b.offset))
	if err != nil {
		panic(err)
	}
	b.p = *(*[]byte)(unsafe.Pointer(&struct {
		p    uintptr
		l, c int
	}{uintptr(b.ptr), PageSize, PageSize}))

	b.offset += PageSize
	return
}

func (b *Writer) Write(p []byte) (n int, err error) {
	c := 0
	lp := len(p)
	var retry int
	for retry = writeTryTimes; retry > 0 && c < lp; retry-- {
		if len(b.p) == 0 {
			err = b.mapper()
			if err != nil {
				return
			}
		}
		n = copy(b.p, p[c:])
		b.p = b.p[n:]
		c += n
	}
	if retry <= 0 {
		err = ReachMaxTryTimes
		return
	}
	return
}

func NewWriter(path string, q *FQueue) (w *Writer, err error) {
	var fd *os.File
	fd, err = os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return
	}
	w = &Writer{
		FQueue: q,
		fd:     fd,
	}
	//set the mapper offset
	if w.WriterOffset%PageSize == 0 {
		w.offset = int64(w.WriterOffset)
	} else {
		offset := (w.WriterOffset/PageSize + 1) * PageSize
		if offset > q.Limit {
			offset = q.Limit
		}
		w.offset = int64(offset)
	}
	err = nil
	return
}

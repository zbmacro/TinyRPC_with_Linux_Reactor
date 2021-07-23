package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer // 写入缓冲区，防止阻塞。Writer 的接口是指针类型的接口
	dec  *gob.Decoder  // Decoder 的接口是指针类型的接口
	enc  *gob.Encoder  // Encoder 的接口是指针类型的接口
}

func (g *GobCodec) Close() error {
	return g.conn.Close() // 关闭连接
}

func (g *GobCodec) ReadHeader(header *Header) error {
	return g.dec.Decode(header)
}

func (g *GobCodec) ReadBody(body interface{}) error {
	return g.dec.Decode(body)
}

func (g *GobCodec) Writer(header *Header, body interface{}) (err error) {
	defer func() {
		_ = g.buf.Flush()
		if err != nil {
			_ = g.Close()
		}
	}()

	if err = g.enc.Encode(header); err != nil {
		log.Println("rpc codec: gob error encoding header: ", err)
		return
	}
	if err = g.enc.Encode(body); err != nil {
		log.Println("rpc codec: gob error encoding body: ", err)
		return
	}
	return
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		enc:  gob.NewEncoder(buf),
		dec:  gob.NewDecoder(conn),
	}
}

func init() {
	Register("gob", NewGobCodec)
}

package codec

import (
	"bufio"
	"encoding/json"
	"io"
)

// json 不能读到错误，不能读到-1

type JsonCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *json.Decoder
	enc  *json.Encoder
}

func (j *JsonCodec) Close() error {
	return j.conn.Close()
}

func (j *JsonCodec) ReadHeader(header *Header) error {
	return j.dec.Decode(header)
}

func (j *JsonCodec) ReadBody(body interface{}) error {
	return j.dec.Decode(body)
}

func (j *JsonCodec) Writer(header *Header, body interface{}) (err error) {
	defer func() {
		e := j.buf.Flush()
		if err == nil {
			err = e
		}
	}()

	if err = j.enc.Encode(header); err != nil {
		return
	}
	if body != nil {
		if err = j.enc.Encode(body); err != nil {
			return
		}
	}
	return
}

var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf:  buf,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(buf),
	}
}

func init() {
	Register("json", NewJsonCodec)
}

package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

type JsonCodec struct {
	conn io.ReadWriteCloser
	buf *bufio.Writer
	encode *json.Encoder
	decode *json.Decoder
}

var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	buf :=bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf: buf,
		decode: json.NewDecoder(conn),
		encode: json.NewEncoder(buf),
	}
}

func (jc *JsonCodec) ReadHeader(h *Header) error {
	return jc.decode.Decode(h)
}

func (jc *JsonCodec) ReadBody(body interface{})error{
	return jc.decode.Decode(body)
}

func (jc *JsonCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = jc.buf.Flush()
		if err !=nil {
			_ =jc.Close()
		}
	}()
	if err := jc.encode.Encode(h); err != nil {
		log.Println("rpc codec: json error encoding header:", err)
		return err
	}
	if err := jc.encode.Encode(body); err != nil {
		log.Println("rpc codec: json error encoding body:", err)
		return err
	}
	return nil
}

func (jc *JsonCodec) Close() error {
	return jc.conn.Close()
}
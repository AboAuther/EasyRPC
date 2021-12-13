package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn   io.ReadWriteCloser
	buf    *bufio.Writer
	decode *gob.Decoder //Decode 解码
	encode *gob.Encoder //Encode 编码
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn:   conn,
		buf:    buf,
		decode: gob.NewDecoder(conn),
		encode: gob.NewEncoder(buf),
	}
}

//ReadHeader 解码header
func (c *GobCodec) ReadHeader(h *Header) error {
	return c.decode.Decode(h)
}

//ReadBody 解码，并存储到body
func (c *GobCodec) ReadBody(body interface{}) error {
	return c.decode.Decode(body)
}

//Write 将header和body编码
func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err := c.encode.Encode(h); err != nil {
		log.Println("rpc codec: gob error encoding header:", err)
		return err
	}
	if err := c.encode.Encode(body); err != nil {
		log.Println("rpc codec: gob error encoding body:", err)
		return err
	}
	return nil
}

//Close conn实例关闭
func (c *GobCodec) Close() error {
	return c.conn.Close()
}

package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

var _ Codec = (*GobCodec)(nil)

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn) // 往conn写数据的buffer
	return &GobCodec{
		conn: conn,                 // 连接
		buf:  buf,                  // buffer
		dec:  gob.NewDecoder(conn), // 解码器，从conn获取数据
		enc:  gob.NewEncoder(buf),  // 编码器，往buf里面写数据
	}
}

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error) {
	// 把buf writer 中的数据从conn发送到出去
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()

	// 编码header，保存在conn的buf writer
	err = c.enc.Encode(h)
	if err != nil {
		log.Println("RPC codec: gob encoding header err:", err)
		return err
	}

	// 编码body，保存在conn的buf writer
	err = c.enc.Encode(body)
	if err != nil {
		log.Println("RPC codec: gob encoding header err:", err)
		return err
	}

	return nil

}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}

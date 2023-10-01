package coder

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCoder struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

var _ Coder = (*GobCoder)(nil)

// NewGobCoder 把conn包装成一个coder
func NewGobCoder(conn io.ReadWriteCloser) Coder {
	buf := bufio.NewWriter(conn) // 往conn写数据的buffer
	return &GobCoder{
		conn: conn,                 // 连接
		buf:  buf,                  // buffer
		dec:  gob.NewDecoder(conn), // 解码器，从conn获取数据
		enc:  gob.NewEncoder(buf),  // 编码器，往buf里面写数据
	}
}

func (c *GobCoder) Close() error {
	return c.conn.Close()
}

// ReadHeader 读取数据存在Header中
func (c *GobCoder) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

// ReadBody 读取数据存在Body中
func (c *GobCoder) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

// 编码Header和Body，写到buf中，把buf中的数据发送到conn里
func (c *GobCoder) Write(h *Header, body interface{}) (err error) {
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
		log.Println("RPC coder: gob encoding header err:", err)
		return err
	}

	// 编码body，保存在conn的buf writer
	err = c.enc.Encode(body)
	if err != nil {
		log.Println("RPC coder: gob encoding header err:", err)
		return err
	}

	return nil

}

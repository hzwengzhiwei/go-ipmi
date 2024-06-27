package ipmi

import (
	"fmt"
)

// 34.2 Read FRU Data Command
type ReadFRUDataRequest struct {
	FRUDeviceID uint8
	ReadOffset  uint16
	ReadCount   uint8
}

type ReadFRUDataResponse struct {
	CountReturned uint8
	Data          []byte
}

func (req *ReadFRUDataRequest) Command() Command {
	return CommandReadFRUData
}

func (req *ReadFRUDataRequest) Pack() []byte {
	out := make([]byte, 4)
	packUint8(req.FRUDeviceID, out, 0)
	packUint16L(req.ReadOffset, out, 1)
	packUint8(req.ReadCount, out, 3)
	return out
}

func (res *ReadFRUDataResponse) Unpack(msg []byte) error {
	if len(msg) < 1 {
		return ErrUnpackedDataTooShortWith(len(msg), 1)
	}

	res.CountReturned, _, _ = unpackUint8(msg, 0)
	res.Data, _, _ = unpackBytes(msg, 1, len(msg)-1)
	return nil
}

func (r *ReadFRUDataResponse) CompletionCodes() map[uint8]string {
	return map[uint8]string{
		0x81: "FRU device busy",
	}
}

func (res *ReadFRUDataResponse) Format() string {
	return fmt.Sprintf(`Count returned : %d
Data           : %02x`,
		res.CountReturned,
		res.Data,
	)
}

// The command returns the specified data from the FRU Inventory Info area.
func (c *Client) ReadFRUData(fruDeviceID uint8, readOffset uint16, readCount uint8) (response *ReadFRUDataResponse, err error) {
	request := &ReadFRUDataRequest{
		FRUDeviceID: fruDeviceID,
		ReadOffset:  readOffset,
		ReadCount:   readCount,
	}
	response = &ReadFRUDataResponse{}
	err = c.Exchange(request, response)
	return
}

// readFRUDataByLength reads FRU Data in loop until reaches the specified data length
func (c *Client) readFRUDataByLength(deviceID uint8, offset uint16, length uint16) ([]byte, error) {
	var data []byte
	c.Debugf("Read FRU Data by Length, offset: (%d), length: (%d)\n", offset, length)

	for {
		if length <= 0 {
			break
		}

		res, err := c.tryReadFRUData(deviceID, offset, length)
		if err != nil {
			return nil, fmt.Errorf("tryReadFRUData failed, err: %s", err)
		}
		c.Debug("", res.Format())
		data = append(data, res.Data...)

		length -= uint16(res.CountReturned)
		c.Debugf("left length: %d\n", length)

		// update offset
		offset += uint16(res.CountReturned)
	}

	return data, nil
}

// tryReadFRUData will try to read FRU data with a read count which starts with
// the minimal number of the specified length and the hard-coded 32, if the
// ReadFRUData failed, it try another request with a decreased read count.
func (c *Client) tryReadFRUData(deviceID uint8, readOffset uint16, length uint16) (response *ReadFRUDataResponse, err error) {
	var readCount uint8 = 32
	if length <= uint16(readCount) {
		readCount = uint8(length)
	}

	for {
		if readCount <= 0 {
			return nil, fmt.Errorf("nothing to read")
		}

		c.Debugf("Try Read FRU Data, offset: (%d), count: (%d)\n", readOffset, readCount)
		res, err := c.ReadFRUData(deviceID, readOffset, readCount)
		if err == nil {
			return res, nil
		}

		resErr, ok := err.(*ResponseError)
		if !ok {
			return nil, fmt.Errorf("ReadFRUData failed, err: %s", err)
		}

		cc := resErr.CompletionCode()
		if readFRUDataLength2Big(cc) {
			readCount -= 1
			continue
		} else {
			return nil, fmt.Errorf("ReadFRUData failed, err: %s", err)
		}
	}
}

func readFRUDataLength2Big(cc CompletionCode) bool {
	return cc == CompletionCodeRequestDataLengthInvalid ||
		cc == CompletionCodeRequestDataLengthLimitExceeded ||
		cc == CompletionCodeCannotReturnRequestedDataBytes
}

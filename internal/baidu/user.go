package baidu

import (
	"context"
	"net/url"
)

const (
	VIPOrdinary = 0
	VIPMember   = 1
	VIPSuper    = 2
)

type UserInfo struct {
	Errno       int    `json:"errno"`
	ErrMsg      string `json:"errmsg"`
	BaiduName   string `json:"baidu_name"`
	NetdiskName string `json:"netdisk_name"`
	UK          int64  `json:"uk"`
	VIPType     int    `json:"vip_type"`
}

func (c Client) UserInfo(ctx context.Context) (UserInfo, error) {
	q := url.Values{}
	q.Set("method", "uinfo")
	var out UserInfo
	err := c.getJSON(ctx, c.pan()+"/rest/2.0/xpan/nas", q, &out)
	return out, err
}

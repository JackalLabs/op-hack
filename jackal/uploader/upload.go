package uploader

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/desmos-labs/cosmos-go-wallet/wallet"
	canine "github.com/jackalLabs/canine-chain/v3/app"
	"github.com/jackalLabs/canine-chain/v3/x/storage/types"
	"github.com/jackalLabs/canine-chain/v3/x/storage/utils"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

type IPFSResponse struct {
	Cid string `json:"cid"`
}

func uploadFile(ip string, r io.Reader, merkle []byte, start int64, address string) (string, error) {
	cli := http.DefaultClient

	u, err := url.Parse(ip)
	if err != nil {
		return "", err
	}

	u = u.JoinPath("upload")

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	defer writer.Close()

	err = writer.WriteField("sender", address)
	if err != nil {
		return "", err
	}

	err = writer.WriteField("merkle", hex.EncodeToString(merkle))
	if err != nil {
		return "", err
	}

	err = writer.WriteField("start", fmt.Sprintf("%d", start))
	if err != nil {
		return "", err
	}

	fileWriter, err := writer.CreateFormFile("file", hex.EncodeToString(merkle))
	if err != nil {
		return "", err
	}

	_, err = io.Copy(fileWriter, r)
	if err != nil {
		return "", err
	}
	writer.Close()

	req, _ := http.NewRequest("POST", u.String(), &b)
	req.Header.Add("Content-Type", writer.FormDataContentType())

	res, err := cli.Do(req)
	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	if res.StatusCode != 200 {

		var errRes ErrorResponse

		err := json.NewDecoder(res.Body).Decode(&errRes)
		if err != nil {
			return "", err
		}

		return "", fmt.Errorf("upload failed with code %d | %s", res.StatusCode, errRes.Error)
	}

	var ipfsRes IPFSResponse
	err = json.NewDecoder(res.Body).Decode(&ipfsRes)
	if err != nil {
		return "", err
	}

	return ipfsRes.Cid, nil
}

func DownloadFile(ctx context.Context, merkle []byte, w *wallet.Wallet) ([]byte, error) {

	cl := types.NewQueryClient(w.Client.GRPCConn)
	r, err := cl.FindFile(ctx, &types.QueryFindFile{Merkle: merkle})
	if err != nil {
		return nil, fmt.Errorf("failed to find file %x | %w", merkle, err)
	}
	ips := r.ProviderIps

	for _, ip := range ips {
		url := fmt.Sprintf("%s/download/%x", ip, merkle)

		// Get the data
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("Error downloading file:", err)
			continue
		}
		defer resp.Body.Close()

		// Check server response
		if resp.StatusCode != http.StatusOK {
			fmt.Println("Error: bad status code", resp.StatusCode)
			continue
		}

		// Read the body as bytes
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			continue
		}
		return body, nil
	}

	return nil, fmt.Errorf("could not find the file on any provider")
}

func PostFile(ctx context.Context, queue *Queue, fileData []byte, w *wallet.Wallet) (string, []byte, error) {
	buf := bytes.NewBuffer(fileData)
	treeBuffer := bytes.NewBuffer(buf.Bytes())

	cl := types.NewQueryClient(w.Client.GRPCConn)

	params, err := cl.Params(ctx, &types.QueryParams{})
	if err != nil {
		return "", nil, err
	}

	root, _, _, size, err := utils.BuildTree(treeBuffer, params.Params.ChunkSize)
	if err != nil {
		return "", root, err
	}

	address := w.AccAddress()

	msg := types.NewMsgPostFile(
		address,
		root,
		int64(size),
		40,
		0,
		3,
		"{\"memo\":\"Optimism makes Jackal a very happy protocol!\"}",
	)
	if err := msg.ValidateBasic(); err != nil {
		return "", root, err
	}

	res, err := queue.Post(msg)
	if err != nil {
		return "", root, err
	}
	if res == nil {
		fmt.Println(res.RawLog)
		return "", root, fmt.Errorf(res.RawLog)
	}
	if res.Code != 0 {
		return "", root, fmt.Errorf(res.RawLog)
	}

	var postRes types.MsgPostFileResponse
	resData, err := hex.DecodeString(res.Data)
	if err != nil {
		return "", root, err
	}

	encodingCfg := canine.MakeEncodingConfig()
	var txMsgData sdk.TxMsgData
	err = encodingCfg.Marshaler.Unmarshal(resData, &txMsgData)
	if err != nil {
		return "", root, err
	}

	fmt.Println(txMsgData)
	if len(txMsgData.Data) == 0 {
		return "", root, fmt.Errorf("no message data")
	}

	err = postRes.Unmarshal(txMsgData.Data[0].Data)
	if err != nil {
		return "", root, err
	}

	ips := postRes.ProviderIps
	fmt.Println(ips)

	fmt.Println(res.Code)
	fmt.Println(res.RawLog)
	fmt.Println(res.TxHash)

	c := ""

	ipCount := len(ips)
	randomCount := 3 - ipCount
	for i := 0; i < ipCount; i++ {
		ip := ips[i]
		uploadBuffer := bytes.NewBuffer(buf.Bytes())
		cid, err := uploadFile(ip, uploadBuffer, root, postRes.StartBlock, address)
		if err != nil {
			fmt.Println(err)
			continue
		}
		c = cid
	}
	pageReq := &query.PageRequest{
		Key:        nil,
		Offset:     0,
		Limit:      200,
		CountTotal: false,
		Reverse:    false,
	}
	provReq := types.QueryAllProviders{
		Pagination: pageReq,
	}

	provRes, err := cl.AllProviders(ctx, &provReq)
	if err != nil {
		return c, root, err
	}

	providers := provRes.Providers
	for i, provider := range providers {
		if i > randomCount {
			break
		}
		uploadBuffer := bytes.NewBuffer(buf.Bytes())
		cid, err := uploadFile(provider.Ip, uploadBuffer, root, postRes.StartBlock, address)
		if err != nil {
			fmt.Println(err)
			continue
		}
		if len(c) == 0 {
			c = cid
		}
	}
	return c, root, nil
}

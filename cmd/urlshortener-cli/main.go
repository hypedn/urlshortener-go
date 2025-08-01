package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	proto "github.com/ndajr/urlshortener-go/rpc/proto/urlshortener/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var (
	grpcServerEndpoint = flag.String("grpc-server-endpoint", "localhost:8081", "gRPC server endpoint")
)

const usage = `Usage: urlshortener [flags] <command> <value>

A CLI to interact with the URL shortener service.

Commands:
  shorten <url>    Shortens a long URL.
  get <code>       Retrieves the original URL from a short code.

Flags:
`

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "Error: invalid arguments. Expected a command and a value.")
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]
	value := args[1]

	conn, err := grpc.NewClient(*grpcServerEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: could not connect to server. Make sure the server is running and try again..")
		os.Exit(1)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "Error: failed to close grpc connection.")
			os.Exit(1)
		}
	}()

	client := proto.NewURLShortenerServiceClient(conn)
	ctx := context.Background()

	switch command {
	case "shorten":
		shortenURLCmd(ctx, client, value)
	case "get":
		getURLCmd(ctx, client, value)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command %q\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func shortenURLCmd(ctx context.Context, client proto.URLShortenerServiceClient, originalURL string) {
	res, err := client.ShortenURL(ctx, &proto.ShortenURLRequest{
		OriginalUrl: originalURL,
	})
	if err != nil {
		if s := status.Convert(err); s.Code() == codes.InvalidArgument {
			fmt.Fprintf(os.Stderr, "Error: %s\n", s.Message())
			os.Exit(1)
		}
		log.Fatalf("could not shorten url: %v", err)
	}
	fmt.Printf("shortened url: %s\n", res.ShortCode)
}

func getURLCmd(ctx context.Context, client proto.URLShortenerServiceClient, shortCode string) {
	res, err := client.GetOriginalURL(ctx, &proto.GetOriginalURLRequest{
		ShortCode: shortCode,
	})
	if err != nil {
		if s := status.Convert(err); s.Code() == codes.NotFound {
			fmt.Println("url not found")
			return
		}
		log.Fatalf("could not get url: %v", err)
	}
	fmt.Printf("original url: %s\n", res.OriginalUrl)
}

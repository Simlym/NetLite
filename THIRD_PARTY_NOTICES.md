# Third-party notices

## ddns-go

The native DDNS providers and signing routines were developed with reference to
[jeessy2/ddns-go v6.17.7](https://github.com/jeessy2/ddns-go/tree/v6.17.7), notably
`dns/cloudflare.go`, `dns/dnspod.go`, `dns/tencent_cloud.go`, `dns/alidns.go`, and
`util/tencent_cloud_signer.go`, `util/aliyun_signer*.go`.
NetLite implements its own task scheduling, IP fetching, result reporting and
provider adapters. It does not bundle or launch the ddns-go executable.
The applicable upstream license is retained below.

MIT License

Copyright (c) 2020 jeessy

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## acmez

NetLite uses [mholt/acmez v3.1.2](https://github.com/mholt/acmez/tree/v3.1.2)
as its ACME protocol client. Copyright 2020 Matthew Holt. Licensed under the
Apache License, Version 2.0: https://www.apache.org/licenses/LICENSE-2.0


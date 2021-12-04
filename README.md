# rtpdump

Thanks to [github.com/hdiniz/rtpdump](http://github.com/hdiniz/rtpdump)! Added EVS extracting capability and now supports 802.1q.

The rtpdump extracts media files from RTP streams in pcap format.

## codec support

This program is intended to support usual audio/video codecs used on IMS networks (VoLTE/VoWiFi).  
Therefore, some codecs might be limited to usual scenarios on these networks.

+ AMR - [RFC 4867](https://tools.ietf.org/html/rfc4867)  
  Supports bandwidth-efficient and octet-aligned modes.  
  Single-channel, single-frame per packet only.
+ H264 - [RFC 6184](https://tools.ietf.org/html/rfc6184)  
Supports Single NAL Mode and some Non-Interleaved Mode streams, due to current lack of STAP-A support  


| Payload Type  	| Support      	|
|---------------	|--------------	|
| 1-23 NAL Unit 	| Yes          	|
| 24 STAP-A     	| No - planned 	|
| 25 STAP-B     	| No           	|
| 26 MTAP16     	| No           	|
| 27 MTAP24     	| Yes          	|
| 28 FU-A       	| Yes          	|
| 29 FU-B       	| No           	|

+ EVS - [3GPP TS 26.445](http://www.3gpp.org/DynaReport/26445.htm)  
  Supports EVS Primary Compact Frame.  
  Supports EVS Primary Header-Full format, with one ToC + single frame.  
  Supports EVS Primary 56 bits (Special case).  
  Supports EVS AMR-WB IO SID (Special case).  
  *Not supported EVS Header-Full format with multiple ToC and multiple frames*.  
  *Not supported EVS IO, implementation is in progress, contributions welcome!*.  
+ H263 - [RFC 2190](https://tools.ietf.org/html/rfc2190)  
  *Not yet supported.*


## convert EVS to audio file

Use the decoder provided by 3GPP TS 26.442 or 3GPP TS 26.443 to convert evs-mime storage format to binary 
synthesized audio file in the following way: 

On Windows:
EVS_dec.exe -mime 48 input.evs-mime out_PCM.raw

The source code can be compiled for Linux also. The raw file can be imported to e.g Audacity for listening.



## ipsec support

In order to support dumping VoWiFi media some support for ESP (Encapsulating Security Payload) decryption is present.

| Encryption Algorithm | Support       |
|--------------------- |-------------- |
| 3DES CBC             | Yes           |
| DES CBC              | No - Planned  |
| AES CBC              | No - Planned  |

Keys are read from file 'esp-keys.txt' on the current directory *by default*. One key per file, for example:

[SPI] [Encryption Algorithm] [Key]  
0x00d40016 des3_cbc 0x091199869ec18afd8e38f77eb1252685924937d3921a178e  
0xcb97da43 des3_cbc 0xaaa316cd3fa41daa9afe6e8f42a9ae0ce2bd5128cef5a60f

Global flag `-k` can be used to indicate another key file path. Check `-help`.

## replaying

Its possible to replay a RTP stream, specifying the destination host and port. The stream consumer can be a actual mobile handset or any application that can interpret RTP streams (e.g VLC).

The stream is replayed as is, taking into account the original timestamps in the pcap file and mantaining the original RTP payload type.
It's up to the receiver to interpret the appropriate stream codec.

For example, VLC accepts a SDP input file:
```
v=0
c=IN IP4 127.0.0.1
m=audio 1234 RTP/AVP 99
a=rtpmap:99 AMR/8000
```
> rtpdump play --host localhost --port 1234 [pcap containing amr-nb payload type 99]

## usage

+ rtpdump streams [pcap]  
  displays RTP streams
+ rtpdump dump [pcap]
  dumps a media stream.
+ rtpdump play (--host localhost --port port) [pcap]
  replays a RTP stream over UDP.

## compiling

Checkout [gopacket](https://github.com/google/gopacket).
Linux should be straightforward.  
For Windows, make sure mingw(32/64) toolchain is on PATH for gopacket WinPcap dependency. Install WinPcap on standard location `C:\WpdPack`

## planned features

1. EVS Header-Full format with multiple ToC and multiple frames (TS 26.445 Chapter A.2.2 )


## contributions

Are always appreciated.

package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"spark/sparkos/tea"
)

func main() {
	var (
		inPath  = flag.String("in", "", "Input file (.wav for encode, .tea for decode).")
		outPath = flag.String("out", "", "Output file (.tea for encode, .wav for decode).")
		mode    = flag.String("mode", "encode", "encode|decode.")
		codec   = flag.String("codec", "ima-adpcm", "pcm16|ima-adpcm (encode mode only).")
		spb     = flag.Int("spb", 512, "Samples per block.")
	)
	flag.Parse()

	if *inPath == "" || *outPath == "" {
		fatalf("usage: mktea -mode encode -in in.wav -out out.tea [-codec pcm16|ima-adpcm] [-spb 512]\n       mktea -mode decode -in in.tea -out out.wav")
	}

	switch strings.ToLower(*mode) {
	case "encode":
		if err := encodeWAVToTEA(*inPath, *outPath, *codec, *spb); err != nil {
			fatalf("encode: %v", err)
		}
	case "decode":
		if err := decodeTEAToWAV(*inPath, *outPath); err != nil {
			fatalf("decode: %v", err)
		}
	default:
		fatalf("unknown mode: %s", *mode)
	}
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}

type wavInfo struct {
	sampleRate uint32
	channels   uint16
	bits       uint16
	dataOff    int64
	dataSize   uint32
}

func encodeWAVToTEA(inPath, outPath, codec string, samplesPerBlock int) error {
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()

	wi, err := parseWAV(in)
	if err != nil {
		return err
	}
	if wi.channels != 1 || wi.bits != 16 {
		return fmt.Errorf("wav: only PCM16 mono is supported (got channels=%d bits=%d)", wi.channels, wi.bits)
	}
	if samplesPerBlock <= 0 || samplesPerBlock > 4096 {
		return fmt.Errorf("spb out of range: %d", samplesPerBlock)
	}

	totalSamples := wi.dataSize / 2
	if totalSamples == 0 {
		return fmt.Errorf("wav: empty data")
	}

	var codecID uint8
	var blockSize uint16
	switch strings.ToLower(codec) {
	case "pcm16":
		codecID = tea.CodecPCM16
		blockSize = uint16(samplesPerBlock * 2)
	case "ima-adpcm", "adpcm":
		codecID = tea.CodecIMAADPCM
		blockSize = uint16(3 + (samplesPerBlock-1+1)/2)
	default:
		return fmt.Errorf("unknown codec: %s", codec)
	}
	if blockSize > tea.MaxBlockBytes {
		return fmt.Errorf("block too large: %d > %d", blockSize, tea.MaxBlockBytes)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, 64*1024)
	defer bw.Flush()

	h := tea.Header{
		Magic:           tea.Magic,
		SampleRate:      uint16(wi.sampleRate),
		Channels:        1,
		CodecID:         codecID,
		SamplesPerBlock: uint16(samplesPerBlock),
		BlockSize:       blockSize,
		TotalSamples:    totalSamples,
		Flags:           0,
	}
	if err := tea.WriteHeader(bw, h); err != nil {
		return err
	}

	if _, err := in.Seek(wi.dataOff, io.SeekStart); err != nil {
		return err
	}

	samples := make([]int16, samplesPerBlock)
	block := make([]byte, blockSize)

	remain := totalSamples
	for remain > 0 {
		want := uint32(samplesPerBlock)
		if remain < want {
			want = remain
		}
		if err := readPCM16Samples(in, samples, int(want)); err != nil {
			return err
		}
		if int(want) < samplesPerBlock {
			last := int16(0)
			if want > 0 {
				last = samples[want-1]
			}
			for i := int(want); i < samplesPerBlock; i++ {
				samples[i] = last
			}
		}

		switch codecID {
		case tea.CodecPCM16:
			if err := tea.EncodePCM16Block(samples, samplesPerBlock, block); err != nil {
				return err
			}
		case tea.CodecIMAADPCM:
			if err := tea.EncodeIMAADPCMBlock(samples, samplesPerBlock, block); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported codec id: %d", codecID)
		}

		if _, err := bw.Write(block); err != nil {
			return err
		}
		remain -= want
	}

	return bw.Flush()
}

func decodeTEAToWAV(inPath, outPath string) error {
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()

	dec, err := tea.NewDecoder(in)
	if err != nil {
		return err
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, 64*1024)
	defer bw.Flush()

	// Write placeholder WAV header; patch sizes at end.
	if err := writeWAVHeader(bw, uint32(dec.Header.SampleRate), 1, 16, 0); err != nil {
		return err
	}

	var (
		totalBytes uint32
		outPCM     = make([]int16, int(dec.Header.SamplesPerBlock))
	)

	for {
		n, err := dec.DecodeBlock(outPCM)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			var tmp [2]byte
			binary.LittleEndian.PutUint16(tmp[:], uint16(outPCM[i]))
			if _, err := bw.Write(tmp[:]); err != nil {
				return err
			}
			totalBytes += 2
		}
	}

	if err := bw.Flush(); err != nil {
		return err
	}

	// Patch sizes.
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := writeWAVHeader(out, uint32(dec.Header.SampleRate), 1, 16, totalBytes); err != nil {
		return err
	}
	return nil
}

func parseWAV(r io.ReadSeeker) (*wavInfo, error) {
	var hdr [12]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: bad header")
	}

	var (
		foundFmt  bool
		foundData bool
		wi        wavInfo
	)
	for {
		var ch [8]byte
		_, err := io.ReadFull(r, ch[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		id := string(ch[0:4])
		sz := binary.LittleEndian.Uint32(ch[4:8])

		switch id {
		case "fmt ":
			if sz < 16 {
				return nil, fmt.Errorf("wav: short fmt chunk")
			}
			buf := make([]byte, sz)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, err
			}
			audioFormat := binary.LittleEndian.Uint16(buf[0:2])
			wi.channels = binary.LittleEndian.Uint16(buf[2:4])
			wi.sampleRate = binary.LittleEndian.Uint32(buf[4:8])
			wi.bits = binary.LittleEndian.Uint16(buf[14:16])
			if audioFormat != 1 {
				return nil, fmt.Errorf("wav: only PCM is supported (format=%d)", audioFormat)
			}
			foundFmt = true

		case "data":
			off, _ := r.Seek(0, io.SeekCurrent)
			wi.dataOff = off
			wi.dataSize = sz
			if _, err := r.Seek(int64(sz), io.SeekCurrent); err != nil {
				return nil, err
			}
			foundData = true

		default:
			if _, err := r.Seek(int64(sz), io.SeekCurrent); err != nil {
				return nil, err
			}
		}

		if sz%2 == 1 {
			if _, err := r.Seek(1, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}

	if !foundFmt || !foundData {
		return nil, fmt.Errorf("wav: missing fmt or data chunk")
	}
	return &wi, nil
}

func readPCM16Samples(r io.Reader, dst []int16, n int) error {
	var buf [2]byte
	for i := 0; i < n; i++ {
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return err
		}
		dst[i] = int16(binary.LittleEndian.Uint16(buf[:]))
	}
	return nil
}

func writeWAVHeader(w io.Writer, sampleRate uint32, channels uint16, bits uint16, dataBytes uint32) error {
	blockAlign := channels * (bits / 8)
	byteRate := sampleRate * uint32(blockAlign)
	riffSize := 4 + (8 + 16) + (8 + dataBytes)

	var hdr [44]byte
	copy(hdr[0:4], []byte("RIFF"))
	binary.LittleEndian.PutUint32(hdr[4:8], riffSize)
	copy(hdr[8:12], []byte("WAVE"))

	copy(hdr[12:16], []byte("fmt "))
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1)
	binary.LittleEndian.PutUint16(hdr[22:24], channels)
	binary.LittleEndian.PutUint32(hdr[24:28], sampleRate)
	binary.LittleEndian.PutUint32(hdr[28:32], byteRate)
	binary.LittleEndian.PutUint16(hdr[32:34], blockAlign)
	binary.LittleEndian.PutUint16(hdr[34:36], bits)

	copy(hdr[36:40], []byte("data"))
	binary.LittleEndian.PutUint32(hdr[40:44], dataBytes)

	_, err := w.Write(hdr[:])
	return err
}

package appreleases

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Delta updates: instead of the whole APK a phone downloads a patch from the
// APK it has installed to the new one (bsdiff by Colin Percival, with int32
// suffix arrays to keep memory near 10x the APK size).
//
// Patch format: the magic "KKPATCH1", then one gzip stream of records:
//
//	int64 diffLen, int64 extraLen, int64 seek (big endian)
//	diffLen bytes:  added bytewise (mod 256) to the old file at the old position
//	extraLen bytes: copied as they are
//
// after which the old position moves by seek. The records rebuild the new
// file front to back; the old file is read at any position.

const patchMagic = "KKPATCH1"

// MaxDeltaInput caps the files a patch is made between (memory use).
const MaxDeltaInput = 120 << 20

// Diff writes a patch that turns oldData into newData.
func Diff(oldData, newData []byte, w io.Writer) error {
	if len(oldData) > MaxDeltaInput || len(newData) > MaxDeltaInput {
		return errors.New("delta input too large")
	}
	if _, err := io.WriteString(w, patchMagic); err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(zw, 1<<16)
	if err := bsdiff(oldData, newData, bw); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return zw.Close()
}

// ApplyPatch applies a patch made by Diff to oldData (used by tests; phones apply
// patches themselves).
func ApplyPatch(oldData []byte, patch io.Reader) ([]byte, error) {
	magic := make([]byte, len(patchMagic))
	if _, err := io.ReadFull(patch, magic); err != nil || string(magic) != patchMagic {
		return nil, errors.New("not a delta patch")
	}
	zr, err := gzip.NewReader(patch)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	r := bufio.NewReader(zr)
	var out bytes.Buffer
	var header [24]byte
	oldPos := int64(0)
	for {
		if _, err := io.ReadFull(r, header[:]); err == io.EOF {
			return out.Bytes(), nil
		} else if err != nil {
			return nil, fmt.Errorf("read patch record: %w", err)
		}
		diffLen := int64(binary.BigEndian.Uint64(header[0:]))
		extraLen := int64(binary.BigEndian.Uint64(header[8:]))
		seek := int64(binary.BigEndian.Uint64(header[16:]))
		if diffLen < 0 || extraLen < 0 || oldPos < 0 || oldPos+diffLen > int64(len(oldData)) {
			return nil, errors.New("corrupt patch")
		}
		diff := make([]byte, diffLen)
		if _, err := io.ReadFull(r, diff); err != nil {
			return nil, fmt.Errorf("read patch diff: %w", err)
		}
		for i := range diff {
			diff[i] += oldData[oldPos+int64(i)]
		}
		out.Write(diff)
		if _, err := io.CopyN(&out, r, extraLen); err != nil {
			return nil, fmt.Errorf("read patch extra: %w", err)
		}
		oldPos += diffLen + seek
	}
}

func bsdiff(oldData, newData []byte, w io.Writer) error {
	sa := suffixSort(oldData)
	oldSize, newSize := int32(len(oldData)), int32(len(newData))
	var header [24]byte
	var buf []byte
	var scan, length, pos, lastScan, lastPos, lastOffset int32
	for scan < newSize {
		var oldScore int32
		scan += length
		for scsc := scan; scan < newSize; scan++ {
			length, pos = search(sa, oldData, newData[scan:], 0, oldSize)
			for ; scsc < scan+length; scsc++ {
				if scsc+lastOffset < oldSize && oldData[scsc+lastOffset] == newData[scsc] {
					oldScore++
				}
			}
			if (length == oldScore && length != 0) || length > oldScore+8 {
				break
			}
			if scan+lastOffset < oldSize && oldData[scan+lastOffset] == newData[scan] {
				oldScore--
			}
		}
		if length == oldScore && scan != newSize {
			continue
		}
		// Extend the previous match forwards and this one backwards.
		var s, sf, lenF int32
		for i := int32(0); lastScan+i < scan && lastPos+i < oldSize; {
			if oldData[lastPos+i] == newData[lastScan+i] {
				s++
			}
			i++
			if s*2-i > sf*2-lenF {
				sf, lenF = s, i
			}
		}
		var lenB int32
		if scan < newSize {
			var sb int32
			s = 0
			for i := int32(1); scan >= lastScan+i && pos >= i; i++ {
				if oldData[pos-i] == newData[scan-i] {
					s++
				}
				if s*2-i > sb*2-lenB {
					sb, lenB = s, i
				}
			}
		}
		if lastScan+lenF > scan-lenB {
			overlap := (lastScan + lenF) - (scan - lenB)
			var ss, lenS int32
			s = 0
			for i := int32(0); i < overlap; i++ {
				if newData[lastScan+lenF-overlap+i] == oldData[lastPos+lenF-overlap+i] {
					s++
				}
				if newData[scan-lenB+i] == oldData[pos-lenB+i] {
					s--
				}
				if s > ss {
					ss, lenS = s, i+1
				}
			}
			lenF += lenS - overlap
			lenB -= lenS
		}
		extraLen := (scan - lenB) - (lastScan + lenF)
		binary.BigEndian.PutUint64(header[0:], uint64(int64(lenF)))
		binary.BigEndian.PutUint64(header[8:], uint64(int64(extraLen)))
		binary.BigEndian.PutUint64(header[16:], uint64(int64((pos-lenB)-(lastPos+lenF))))
		if _, err := w.Write(header[:]); err != nil {
			return err
		}
		buf = append(buf[:0], newData[lastScan:lastScan+lenF]...)
		for i := range buf {
			buf[i] -= oldData[lastPos+int32(i)]
		}
		if _, err := w.Write(buf); err != nil {
			return err
		}
		if _, err := w.Write(newData[lastScan+lenF : lastScan+lenF+extraLen]); err != nil {
			return err
		}
		lastScan, lastPos, lastOffset = scan-lenB, pos-lenB, pos-scan
	}
	return nil
}

func matchLen(a, b []byte) int32 {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return int32(i)
}

// search finds the longest match of target in oldData among the suffixes
// sa[st..en]; it returns the match length and its position in oldData.
func search(sa []int32, oldData, target []byte, st, en int32) (int32, int32) {
	for en-st >= 2 {
		x := st + (en-st)/2
		n := min(len(oldData)-int(sa[x]), len(target))
		if bytes.Compare(oldData[sa[x]:int(sa[x])+n], target[:n]) < 0 {
			st = x
		} else {
			en = x
		}
	}
	x := matchLen(oldData[sa[st]:], target)
	y := matchLen(oldData[sa[en]:], target)
	if x > y {
		return x, sa[st]
	}
	return y, sa[en]
}

// suffixSort is bsdiff's qsufsort (Larsson-Sadakane): the suffix array of
// data, with len(data)+1 entries (the empty suffix first).
func suffixSort(data []byte) []int32 {
	n := int32(len(data))
	I := make([]int32, n+1)
	V := make([]int32, n+1)
	var buckets [256]int32
	for _, c := range data {
		buckets[c]++
	}
	for i := 1; i < 256; i++ {
		buckets[i] += buckets[i-1]
	}
	for i := 255; i > 0; i-- {
		buckets[i] = buckets[i-1]
	}
	buckets[0] = 0
	for i, c := range data {
		buckets[c]++
		I[buckets[c]] = int32(i)
	}
	I[0] = n
	for i, c := range data {
		V[i] = buckets[c]
	}
	V[n] = 0
	for i := 1; i < 256; i++ {
		if buckets[i] == buckets[i-1]+1 {
			I[buckets[i]] = -1
		}
	}
	I[0] = -1
	for h := int32(1); I[0] != -(n + 1); h += h {
		var length int32
		i := int32(0)
		for i < n+1 {
			if I[i] < 0 {
				length -= I[i]
				i -= I[i]
			} else {
				if length != 0 {
					I[i-length] = -length
				}
				length = V[I[i]] + 1 - i
				split(I, V, i, length, h)
				i += length
				length = 0
			}
		}
		if length != 0 {
			I[i-length] = -length
		}
	}
	for i := int32(0); i < n+1; i++ {
		I[V[i]] = i
	}
	return I
}

func split(I, V []int32, start, length, h int32) {
	if length < 16 {
		var j int32
		for k := start; k < start+length; k += j {
			j = 1
			x := V[I[k]+h]
			for i := int32(1); k+i < start+length; i++ {
				if V[I[k+i]+h] < x {
					x = V[I[k+i]+h]
					j = 0
				}
				if V[I[k+i]+h] == x {
					I[k+j], I[k+i] = I[k+i], I[k+j]
					j++
				}
			}
			for i := int32(0); i < j; i++ {
				V[I[k+i]] = k + j - 1
			}
			if j == 1 {
				I[k] = -1
			}
		}
		return
	}
	x := V[I[start+length/2]+h]
	var jj, kk int32
	for i := start; i < start+length; i++ {
		if V[I[i]+h] < x {
			jj++
		}
		if V[I[i]+h] == x {
			kk++
		}
	}
	jj += start
	kk += jj
	i, j, k := start, int32(0), int32(0)
	for i < jj {
		switch v := V[I[i]+h]; {
		case v < x:
			i++
		case v == x:
			I[i], I[jj+j] = I[jj+j], I[i]
			j++
		default:
			I[i], I[kk+k] = I[kk+k], I[i]
			k++
		}
	}
	for jj+j < kk {
		if V[I[jj+j]+h] == x {
			j++
		} else {
			I[jj+j], I[kk+k] = I[kk+k], I[jj+j]
			k++
		}
	}
	if jj > start {
		split(I, V, start, jj-start, h)
	}
	for i := int32(0); i < kk-jj; i++ {
		V[I[jj+i]] = kk - 1
	}
	if jj == kk-1 {
		I[jj] = -1
	}
	if start+length > kk {
		split(I, V, kk, start+length-kk, h)
	}
}

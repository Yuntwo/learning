package concurrency

import (
	"fmt"
	"sync"
	"testing"
)

func TestChannelBiNumber(t *testing.T) {
	// 用两个 goroutine 交替打印 1~100：
	// oddTurn 控制奇数 goroutine，evenTurn 控制偶数 goroutine。
	oddTurn := make(chan struct{})
	evenTurn := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		for i := 1; i <= 99; i += 2 {
			<-oddTurn
			fmt.Println(i)
			evenTurn <- struct{}{}
		}
	}()

	go func() {
		defer wg.Done()

		for i := 2; i <= 100; i += 2 {
			<-evenTurn
			fmt.Println(i)

			// 打印到 100 就结束，不再继续唤醒奇数 goroutine。
			if i < 100 {
				oddTurn <- struct{}{}
			}
		}
	}()

	// 先让奇数 goroutine 打印 1，之后两个 goroutine 交替进行。
	oddTurn <- struct{}{}
	wg.Wait()
}

func TestSlice(t *testing.T) {
	// make([]int, 3) 会先创建一个长度为 3 的切片，里面已经有 3 个零值元素。
	slice := make([]int, 3)

	// append 不是“覆盖原来的 3 个位置”，而是追加到末尾，
	// 所以结果会变成 [0 0 0 1 2 3]。
	slice = append(slice, 1, 2, 3)
	fmt.Println(slice)
}

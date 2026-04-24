package transpile

func lowerAsyncModuleFileIR(packageName string) goFileIR {
	fileIR := lowerGoFileIR(packageName, map[string]string{
		helperImportPath: helperImportAlias,
		"sync":           "sync",
	})
	fileIR.Decls = append(fileIR.Decls,
		goDeclIR{Source: `type fiberState[T any] struct {
	wg sync.WaitGroup
	result T
}`},
		goDeclIR{Source: `type Fiber[T any] struct {
	Result T
	Wg any
}`},
		goDeclIR{Source: `func (self Fiber[T]) fiberHandle() any {
	return self.Wg
}`},
		goDeclIR{Source: `func (self Fiber[T]) Get() T {
	self.Join()
	return fiberGet[T](self.Wg)
}`},
		goDeclIR{Source: `func (self Fiber[T]) Join() {
	fiberWait(self.Wg)
}`},
		goDeclIR{Source: `func Sleep(ms int) {
	_, err := ardgo.CallExtern("Sleep", ms)
	if err != nil {
		panic(err)
	}
}`},
		goDeclIR{Source: `func Start(do func()) Fiber[struct{}] {
	state := &fiberState[struct{}]{}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		do()
	}()
	return Fiber[struct{}]{Wg: state}
}`},
		goDeclIR{Source: `func Eval[T any](do func() T) Fiber[T] {
	state := &fiberState[T]{}
	state.wg.Add(1)
	go func() {
		defer state.wg.Done()
		state.result = do()
	}()
	return Fiber[T]{Wg: state}
}`},
		goDeclIR{Source: `func Join[T any](fibers []Fiber[T]) {
	for _, fiber := range fibers {
		fiberWait(fiber.Wg)
	}
}`},
		goDeclIR{Source: `func JoinAny(fibers []any) {
	for _, fiber := range fibers {
		handleProvider, ok := fiber.(interface{ fiberHandle() any })
		if !ok {
			panic("unexpected async fiber")
		}
		fiberWait(handleProvider.fiberHandle())
	}
}`},
		goDeclIR{Source: `type fiberWaiter interface {
	wait()
}`},
		goDeclIR{Source: `type fiberGetter[T any] interface {
	get() T
}`},
		goDeclIR{Source: `func (state *fiberState[T]) wait() {
	state.wg.Wait()
}`},
		goDeclIR{Source: `func (state *fiberState[T]) get() T {
	state.wg.Wait()
	return state.result
}`},
		goDeclIR{Source: `func fiberWait(handle any) {
	if waiter, ok := handle.(fiberWaiter); ok {
		waiter.wait()
		return
	}
	panic("unexpected async fiber handle")
}`},
		goDeclIR{Source: `func fiberGet[T any](handle any) T {
	if getter, ok := handle.(fiberGetter[T]); ok {
		return getter.get()
	}
	panic("unexpected async fiber handle")
}`},
	)
	return fileIR
}

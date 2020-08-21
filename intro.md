dscope 是一个在 go 语言里模拟动态作用域机制的库

## 类型与值的声明

```go
func New(...any) Scope
```

一个 Scope 表示一组类型到值的映射。Scope 是不可变的，所有类型与值关系作为 New 方法的参数传入。

New 方法的参数是函数，这些函数可以返回多个值，每个值的类型，都分别作为类型与值映射中的键，函数实际返回的值，作为映射中的值。

例：

```go
scope := New(
  func() (int, string) {
    return 42, "42"
  },
  func() float64 {
    return 42.42
  },
)
```

定义了三个映射：
* int -> 42
* string -> "42"
* float64 -> 42.42

一个类型唯一对应一个值，不同的值需要使用不同的类型。

例：

```go
type (
  IntA int
  IntB int
  IntC int
)
scope := New(
  func() (IntA, IntB, IntC) {
    return 1, 2, 3
  },
)
```

IntA, IntB, IntC 的基础类型都是 int，但它们是不同的类型，分别映射到 1, 2, 3

## 根据类型获取值

在 New 函数里声明的类型与值的映射，可以通过多种方式获取值。
定义映射的函数会在第一次获取的时候调用，并保存返回的值。多次获取同一个类型的值，返回的值都与第一次获取的相同。

### Assign

```go
func (s Scope) Assign(...any)
```

Scope 的 Assign 方法，参数是指向变量的指针，根据各个变量的类型，获取相应类型的值，并赋值到该变量。
如果传入的类型不存在，将抛出 panic。

例：

```go
var i int
var s string
var f float64

scope.Assign(&i, &s, &f)
```

i、s、f 会被赋值为 42, "42", 42.42

### Get

```go
func (s Scope) Get(reflect.Type) (reflect.Type, bool)
```

Get 方法用于获取以传入的 reflect.Type 为键的值。
如果传入的类型不存在，第二个返回值将为 false

例：

```go
v, ok := scope.Get(reflect.TypeOf((*int)(nil)).Elem())
// ok == true
// v.Int() == 42
```

### Call

```go
func (s Scope) Call(fn any, ret ...any) []reflect.Value
```

Call 方法将调用传入的函数，并以 []reflect.Value 返回该函数的返回值。
如果参数里传入了额外的指向变量的指针，该函数的返回值将赋值给相应类型的变量。

例：

```go
var n int
rets := scope.Call(
  func(i int) int {
    return i * 2
  },
  &n,
)
// n == 42 * 2
// rets[0].Int() == 42 * 2
```

### CallValue

```go
func (s Scope) CallValue(reflect.Value, ret ...any) []reflect.Value
```

CallValue 与 Call 作用一样，区别是函数以 reflect.Value 类型传入 

## 带依赖关系的类型与值的声明

New 方法传入的声明函数，可以带有参数，这些参数的类型，是返回的类型所依赖的类型。

例：

```go
scope := New(
  func() int {
    return 42
  },
  func(i int) string {
    return fmt.Sprintf("%d", i * 2)
  },
  func(i int, s string) float64 {
    return float64(i + len(s))
  },
)
```

定义的映射为：
* int -> 42
* string -> "84"，依赖 int
* float64 -> 44，依赖 int 和 string

依赖关系不允许有环，例：

```go
scope := New(
  func(i int) string {
    return fmt.Sprintf("%d", i)
  },
  func(s string) int {
    return len(s)
  },
)
```

string 类型依赖 int 类型，而 int 类型又依赖 string 类型，则构成了环状依赖，New 调用将 panic。

如果依赖的类型没有在同一 Scope 定义，则会抛出 panic。

## 子作用域

```go
func (s Scope) Sub(...any) Scope
```

Scope 的 Sub 方法，可以根据传入的定义函数，调整一些映射关系，并返回一个新的 Scope 对象。
新旧 Scope 对象的映射关系各自独立。

例：

```go
scope := New(
  func() int {
    return 42
  },
)
sub := scope.Sub(
  func() int {
    return 24
  },
)
var i1, i2 int
sub.Assign(&i2)
// i2 == 24
scope.Assign(&i1)
// i1 == 42
```

sub 子作用域重新定义了 int 类型的值为 24，所以通过 sub 获取的 i2 的值为 24。
同时 scope 的 int 类型的值仍为 42。

子作用域重新定义了的类型，如果是其他类型的依赖，则定义函数会重新调用，以反映依赖关系的改变。

例：

```go
scope := New(
  func() int {
    return 42
  },
  func(i int) float64 {
    return i * 2
  },
)
sub := scope.Sub(
  func() int {
    return 24
  },
)
var f float64
sub.Assign(&f)
// f == 48
var f2 float64
scope.Assign(&f2)
// f2 == 84
```

其中 float64 依赖 int，而 sub 重新定义 int 为 24，则通过 sub 获取的 float64，也会重新计算为 24 * 2。
scope 的 float64 不受 sub 的定义影响，仍为 84。

## 应用场景

### 依赖注入 TODO

### 按需初始化 TODO

## 使用技巧

### 参数化构造 TODO

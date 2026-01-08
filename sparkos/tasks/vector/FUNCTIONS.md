# Vector: полный справочник функций

Этот документ описывает все команды и встроенные функции приложения `Vector` (REPL + плоттер).

## 1) REPL: выражения и типы

### Типы значений

- **Число**: float64 или рациональное (`:exact` режим). Вывод настраивается `:prec N`.
- **Complex**: комплексные числа (`i` задан по умолчанию).
- **Array**: вектор `[]float64` (скаляры на сетке, результаты численных процедур и т.п.).
- **Matrix**: матрица `rows x cols` (row-major).
- **Expr**: отложенное выражение (AST), получаемое через `expr(...)`, `simp(...)`, `diff(...)`, `series(...)`, `expand(...)` и т.п.

### Присваивания и функции

- `a = <expr>` — присваивание переменной.
- `f(x) = <expr>` — пользовательская функция.
- `f(<value>)` — вызов пользовательской функции (ровно 1 аргумент).

### Управляющие/служебные выражения

- `expr(x)` → `Expr` (не вычисляет, возвращает AST).
- `eval(expr)` → вычисляет `Expr` в текущем окружении.
- `simp(expr)` → `Expr` (упрощение).
- `diff(expr, x)` → `Expr` (производная по идентификатору).

### Логика/условия

- `if(cond, a, b)` → `a` или `b` (cond — число/комплекс, `0`=false, иначе true).
- `where(cond, value)` → `value` или `NaN` (удобно для масок/плотов).
- `and(a, b)`, `or(a, b)`, `not(a)` → `0/1`.

### Режимы вычисления

- `numeric(expr)` → вычисляет `expr` как float (временно `modeFloat`).
- `exact(expr)` → вычисляет `expr` как rational (временно `modeExact`).
- `time(expr)` → вычисляет `expr` и выставляет `_time_ms` (в миллисекундах).
- `size(expr)` → приблизительный размер AST (кол-во узлов).

## 2) CAS/полиномы (AST-операции)

Эти функции обрабатываются до вычисления аргументов (можно использовать свободные переменные).

- `expand(expr)` → `Expr`: раскрытие скобок (для `*` над `+/-` и `(a+b)^n` при малом n).
- `series(expr, x, a, n)` → `Expr`: ряд Тейлора в точке `a` до степени `n` (0..64).
- `horner(expr, x)` → `Expr`: перевод полинома в форму Горнера.
- `degree(expr, x)` → `Number`: степень полинома.
- `coeff(expr, x, n)` → `Number`: коэффициент при `x^n`.
- `collect(expr, x)` → `Expr`: сборка полинома в форму Горнера.

Полиномиальные операции над рациональными коэффициентами:

- `gcd(f, g[, x])` → `Expr`: НОД полиномов (монический).
- `lcm(f, g[, x])` → `Expr`: НОК полиномов (монический).
- `resultant(f, g, x)` → `Number`: результант (через Sylvester matrix, степени до 16).
- `factor(expr[, x])` → `Expr`: пытается разложить **целочисленные** полиномы на линейные множители по рациональным корням (иначе возвращает как есть).

## 3) Численные методы

Функции ниже ожидают `expr(...)` аргументы:

- `newton(expr, x0[, tol[, maxIter]])` → `Number`: корень `f(x)=0` (Ньютон).
- `bisection(expr, a, b[, tol[, maxIter]])` → `Number`: корень на отрезке с разными знаками.
- `secant(expr, x0, x1[, tol[, maxIter]])` → `Number`: корень методом секущих.
- `diff_num(expr, x[, h])` → `Number`: численная производная (центральная разность).
- `integrate_num(expr, a, b[, method[, n]])` → `Number`: интеграл по отрезку.
  - `method=0` трапеции, `method=1` Симпсон.
  - если 4-й аргумент > 1, трактуется как `n` (удобство `integrate_num(f,a,b,n)`).
- `interp(data, x)` → `Number`: линейная интерполяция.
  - `data` как массив `[y0,y1,...]` или матрица `Nx2` `[x,y]`.

## 4) Статистика и свёртка

Агрегаты для `Array` (и для `Matrix` — по всем элементам):

- `len(a)` → `Number`
- `sum(a)` → `Number`
- `avg(a)` / `mean(a)` → `Number`
- `min(a)` / `max(a)` → `Number`
- `median(a)` → `Number`
- `variance(a)` → `Number` (population variance)
- `std(a)` → `Number`

Пары массивов:

- `cov(x, y)` → `Number` (population covariance)
- `corr(x, y)` → `Number` (correlation)

Гистограмма:

- `hist(data, bins)` → `Matrix(bins x 2)` `[center,count]`.

Свёртка:

- `convolve(a, b)` → `Array` дискретная свёртка.

## 5) Плоттер: генераторы серий

2D параметрическая кривая:

- `param(x(t), y(t), tmin, tmax[, n])` → `Matrix(N x 2)` (автоплотится как серия).

Неявные/контуры/поля:

- `implicit(expr, xmin, xmax, ymin, ymax[, n])` → `Matrix(N x 2)` сегменты уровня `0` (NaN-разрывы).
- `contour(expr, levels, xmin, xmax, ymin, ymax[, n])` → `Matrix(N x 2)` сегменты уровней.
  - `levels` — число или массив чисел.
- `vectorfield(f, g, xmin, xmax, ymin, ymax[, n])` → `Matrix(N x 2)` набор отрезков (стрелок) для поля.

Плоскость как выражение поверхности:

- `plane(n, d)` или `plane(p0, p1, p2)` → `Expr` для `z(x,y)`.

## 6) Линейная алгебра / матрицы

Создание/формы:

- `zeros(rows, cols)` → `Matrix`
- `ones(rows, cols)` → `Matrix`
- `eye(n)` → `Matrix`
- `reshape(xs, rows, cols)` → `Matrix` (xs — `Array` или `Matrix`)
- `shape(A)` → `Array [rows, cols]`
- `flatten(A)` → `Array`

Доступ:

- `get(v, i)` / `set(v, i, value)` для `Array` (1-based индекс)
- `get(A, row, col)` / `set(A, row, col, value)` для `Matrix` (1-based индексы)
- `row(A, row)` / `col(A, col)` → `Array`
- `diag(A)` → `Array`

Операции:

- `det(A)` → `Number`
- `inv(A)` → `Matrix`
- `trace(A)` → `Number`
- `norm(A)` → `Number` (Frobenius)
- `T(A)` / `transpose(A)` → `Matrix`
- `solve(A, b)` → `Array` или `Matrix` (решение `A*x=b`).
- `qr(A)` → `Q` и выставляет `_Q`, `_R`.
- `svd(A)` → `s` и выставляет `_U`, `_V`, `_S`.

## 7) Векторы

Конструкторы:

- `vec2(x, y)`, `vec3(x, y, z)`, `vec4(x, y, z, w)` → `Array`

Компоненты:

- `x(v)`, `y(v)`, `z(v)`, `w(v)` → скалярная компонента (не путать с переменными `x`, `y`).

Операции:

- `dot(a, b)` → `Number`
- `cross(a, b)` → `Array` (только vec3)
- `mag(v)` / `norm(v)` → `Number`
- `unit(v)` / `normalize(v)` → `Array`
- `dist(a, b)` → `Number`
- `angle(a, b)` → `Number`
- `proj(a, b)` → `Array`
- `outer(a, b)` → `Matrix`
- `lerp(a, b, t)` → `Array`

## 8) Полиномы (коэффициенты)

- `polyval(coeffs, x)` → `Number` или `Complex` (coeffs: `[c0..cn]`).
- `polyfit(data, n)` / `polyfit(x, y, n)` → `Array coeffs`.
- `roots(coeffs)` → `Matrix(N x 2)` (Re/Im комплексных корней).

Также есть численный поиск корней по интервалу:

- `roots(expr, xmin, xmax[, n])` → `Array` приближённых корней.

## 9) Комплексные числа

Константа:

- `i` задано по умолчанию.

Преобразования:

- `re(z)`, `im(z)`, `arg(z)`, `conj(z)` → `Number/Complex`
- `polar(z)` → `Array [r,phi]`
- `rect(r, phi)` → `Complex`

Для `Complex` также доступны: `abs`, `sqrt`, `exp`, `ln/log`, `sin`, `cos`, `tan`.

## 10) Скалярная математика (Number)

Тригонометрия:

- `sin`, `cos`, `tan`, `asin`, `acos`, `atan`, `atan2`, `cot`, `sec`, `csc`

Гиперболические:

- `sinh`, `cosh`, `tanh`, `asinh`, `acosh`, `atanh`

Экспоненты/логи:

- `exp`, `expm1`, `exp2`, `ln`, `log`, `log10`, `log2`, `log1p`

Степени/корни:

- `pow(a,b)`, `sqrt`, `cbrt`, `hypot(a,b)`

Округления:

- `floor`, `ceil`, `trunc`, `round`

Прочее:

- `abs`, `sign`, `copysign`, `mod`
- `rad`, `deg`
- `clamp(x, lo, hi)`, `saturate(x)`
- `sq(x)`, `cube(x)`
- `min(...)`, `max(...)`, `sum(...)`, `avg(...)`, `mean(...)`

Элементно-ориентированные версии этих функций применимы к `Array` и `Matrix`, если функция определена в `unaryArrayBuiltins`.

## 11) Команды интерфейса

### Команды REPL (начинаются с `:`)

- `:save NAME|/path/file.vnb`
- `:load NAME|/path/file.vnb`
- `:notebooks`
- `:new` (очистить notebook)
- `:term` / `:plot` / `:stack`
- `:help` (переключить help)
- `:exact` / `:float`
- `:prec N`
- `:plotclear` / `:plots` / `:plotdel N`
- `:x A B` / `:y A B` / `:view xmin xmax ymin ymax`
- `:clear` (очистить вывод)

` :plot` дополнительно принудительно добавляет график из текущего выражения/`y`.

### Сервисные команды (начинаются с `$`)

- `$help`, `$about`, `$clear`
- `$plotdim 2|3`
- `$plotcolor [0|1|2]` или `$plotcolor mono|height|pos`
- `$resetview`, `$autoscale`
- `$vars`, `$funcs`
- `$del name`

## 12) Горячие клавиши

- `F1/F2/F3` — вкладки terminal/plot/stack
- `Ctrl+G` — перейти в plot из REPL
- `Tab` — autocomplete в REPL; в 3D plot — toggle XYZ axes
- `q` — выход (вне REPL); `Esc` — выход всегда
- `H` — toggle help (вне REPL), в REPL используйте `:help`

Plot:

- 2D: стрелки — pan
- 3D: стрелки — rotate
- `+/-`, `PgUp/PgDn` — zoom
- `z` — шаг зума
- `a` — autoscale
- `c` — обратно в REPL


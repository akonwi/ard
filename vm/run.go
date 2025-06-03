package vm

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
)

var void = &object{nil, checker.Void}

// compareKey is a wrapper around an object to use for map keys
// enabling proper equality comparison
type compareKey struct {
	obj *object
	// Store a string representation for hashability
	strKey string
}

func Run(program *checker.Program) (val any, err error) {
	defer func() {
		if r := recover(); r != nil {
			if msg, ok := r.(string); ok {
				err = errors.New(msg)
			} else {
				panic(r)
			}
		}
	}()

	vm := New()
	for _, statement := range program.Statements {
		vm.result = *vm.do(statement)
	}
	if r, isResult := vm.result.raw.(_result); isResult {
		return r.raw.raw, nil
	}
	return vm.result.raw, nil
}

func (vm *VM) do(stmt checker.Statement) *object {
	if stmt.Break {
		vm.scope._break()
		return void
	}
	if stmt.Expr != nil {
		return vm.eval(stmt.Expr)
	}

	switch s := stmt.Stmt.(type) {
	case *checker.Enum:
		return void
	case *checker.StructDef:
		return void
	case *checker.VariableDef:
		val := vm.eval(s.Value)
		// can be broken by `try`
		if vm.scope.broken {
			return val
		}
		if !s.Mutable {
			original := val.raw
			var copy any = new(any)
			copy = original
			val.raw = copy
		}
		vm.scope.add(s.Name, val)
		return void
	case *checker.Reassignment:
		target := vm.eval(s.Target)
		val := vm.eval(s.Value)
		target.raw = val.raw
		return void
	case *checker.ForLoop:
		init := func() { vm.do(checker.Statement{Stmt: s.Init}) }
		update := func() { vm.do(checker.Statement{Stmt: s.Update}) }
		for init(); vm.eval(s.Condition).raw.(bool); update() {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	case *checker.ForIntRange:
		i := vm.eval(s.Start).raw.(int)
		end := vm.eval(s.End).raw.(int)
		for i <= end {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{i, checker.Int})
			})
			if broke {
				break
			}
			i++
		}
		return void
	case *checker.ForInStr:
		val := vm.eval(s.Value).raw.(string)
		for _, c := range val {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, &object{string(c), checker.Str})
			})
			if broke {
				break
			}
		}
		return void
	case *checker.ForInList:
		val := vm.eval(s.List).raw.([]*object)
		for i := range val {
			_, broke := vm.evalBlock(s.Body, func() {
				vm.scope.breakable = true
				vm.scope.add(s.Cursor, val[i])
			})
			if broke {
				break
			}
		}
		return void
	case *checker.WhileLoop:
		for vm.eval(s.Condition).raw.(bool) {
			_, broke := vm.evalBlock(s.Body, func() { vm.scope.breakable = true })
			if broke {
				break
			}
		}
		return void
	case nil:
		return void
	default:
		panic(fmt.Errorf("Unimplemented statement: %T", s))
	}
}

func (vm *VM) eval(expr checker.Expression) *object {
	switch e := expr.(type) {
	case *checker.StrLiteral:
		return &object{e.Value, e.Type()}
	case *checker.BoolLiteral:
		return &object{e.Value, e.Type()}
	case *checker.IntLiteral:
		return &object{e.Value, e.Type()}
	case *checker.FloatLiteral:
		return &object{e.Value, e.Type()}
	case *checker.TemplateStr:
		sb := strings.Builder{}
		for i := range e.Chunks {
			// chunks implement Str::ToString
			chunk := vm.eval(&checker.InstanceMethod{
				Subject: e.Chunks[i],
				Method: &checker.FunctionCall{
					Name: "to_str",
					Args: []checker.Expression{},
				},
			}).raw.(string)
			sb.WriteString(chunk)
		}
		return &object{sb.String(), checker.Str}
	case *checker.Variable:
		val, ok := vm.scope.get(e.Name())
		if !ok {
			panic(fmt.Errorf("variable not found: %s", e.Name()))
		}
		return val
	case *checker.Not:
		val := vm.eval(e.Value)
		return &object{!val.raw.(bool), val._type}

	case *checker.Negation:
		val := vm.eval(e.Value)
		if num, isInt := val.raw.(int); isInt {
			return &object{-num, val._type}
		}
		return &object{-val.raw.(float64), val._type}
	case *checker.StrAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(string) + right.raw.(string),
			left._type,
		}
	case *checker.IntAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) + right.raw.(int),
			left._type,
		}
	case *checker.IntSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) - right.raw.(int),
			left._type,
		}
	case *checker.IntMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) * right.raw.(int),
			left._type,
		}
	case *checker.IntDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) / right.raw.(int),
			left._type,
		}
	case *checker.IntModulo:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) % right.raw.(int),
			left._type,
		}
	case *checker.IntGreater:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) > right.raw.(int),
			checker.Bool,
		}
	case *checker.IntGreaterEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) >= right.raw.(int),
			checker.Bool,
		}
	case *checker.IntLess:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) < right.raw.(int),
			checker.Bool,
		}
	case *checker.IntLessEqual:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(int) <= right.raw.(int),
			checker.Bool,
		}
	case *checker.FloatDivision:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) / right.raw.(float64),
			left._type,
		}
	case *checker.FloatMultiplication:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) * right.raw.(float64),
			left._type,
		}
	case *checker.FloatSubtraction:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) - right.raw.(float64),
			left._type,
		}
	case *checker.FloatAddition:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{
			left.raw.(float64) + right.raw.(float64),
			left._type,
		}
	case *checker.Equality:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw == right.raw, checker.Bool}
	case *checker.And:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) && right.raw.(bool), checker.Bool}
	case *checker.Or:
		left, right := vm.eval(e.Left), vm.eval(e.Right)
		return &object{left.raw.(bool) || right.raw.(bool), checker.Bool}
	case *checker.If:
		if cond := vm.eval(e.Condition); cond.raw.(bool) {
			res, _ := vm.evalBlock(e.Body, nil)
			return res
		}
		if e.ElseIf != nil && vm.eval(e.ElseIf.Condition).raw.(bool) {
			res, _ := vm.evalBlock(e.ElseIf.Body, nil)
			return res
		}
		if e.Else != nil {
			res, _ := vm.evalBlock(e.Else, nil)
			return res
		}
		return void
	case *checker.FunctionDef:
		raw := func(args ...*object) *object {
			res, _ := vm.evalBlock(e.Body, func() {
				for i := range args {
					vm.scope.add(e.Parameters[i].Name, args[i])
				}
			})
			return res
		}
		obj := &object{raw, e.Type()}
		vm.scope.add(e.Name, obj)
		return obj
	case *checker.Panic:
		msg := vm.eval(e.Message)
		panic(fmt.Sprintf("panic at %s:\n%s", e.GetLocation().Start, msg.raw))
	case *checker.FunctionCall:
		sig, ok := vm.scope.get(e.Name)
		if !ok {
			panic(fmt.Errorf("Undefined: %s", e.Name))
		}
		fn, ok := sig.raw.(func(args ...*object) *object)
		if !ok {
			panic(fmt.Errorf("Not a function: %s: %s", e.Name, sig._type))
		}

		args := make([]*object, len(e.Args))
		for i := range e.Args {
			args[i] = vm.eval(e.Args[i])
		}

		return fn(args...)
	case *checker.InstanceProperty:
		{
			subj := vm.eval(e.Subject)
			if _, ok := subj._type.(*checker.StructDef); ok {
				raw := subj.raw.(map[string]*object)
				return raw[e.Property]
			}

			switch subj._type {
			case checker.Str:
				return vm.evalStrProperty(subj, e.Property)
			default:
				panic(fmt.Errorf("Unimplemented instance property: %s.%s", subj._type, e.Property))
			}
		}
	case *checker.InstanceMethod:
		{
			subj := vm.eval(e.Subject)
			return vm.evalInstanceMethod(subj, e)
		}
	case *checker.PackageFunctionCall:
		{
			if e.Package == "ard/ints" {
				switch e.Call.Name {
				case "from_str":
					input := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if num, err := strconv.Atoi(input); err == nil {
						res.raw = num
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: Int::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/float" {
				switch e.Call.Name {
				case "from_int":
					input := vm.eval(e.Call.Args[0]).raw.(int)
					return &object{float64(input), e.Call.Type()}
				case "from_str":
					input := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if num, err := strconv.ParseFloat(input, 64); err == nil {
						res.raw = num
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: Float::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/fs" {
				switch e.Call.Name {
				case "append":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					content := vm.eval(e.Call.Args[1]).raw.(string)
					res := &object{false, e.Call.Type()}
					if file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
						if _, err := file.WriteString(content); err == nil {
							res.raw = true
						}
						file.Close()
					}
					return res
				case "create_file":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if file, err := os.Create(path); err == nil {
						file.Close()
						res.raw = true
					}
					return res
				case "delete":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if err := os.Remove(path); err == nil {
						res.raw = true
					}
					return res
				case "exists":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					res := &object{false, e.Call.Type()}
					if _, err := os.Stat(path); err == nil {
						res.raw = true
					}
					return res
				case "read":
					path := vm.eval(e.Call.Args[0]).raw.(string)

					res := &object{nil, e.Call.Type()}
					if content, err := os.ReadFile(path); err == nil {
						res.raw = string(content)
					}
					return res
				case "write":
					path := vm.eval(e.Call.Args[0]).raw.(string)
					content := vm.eval(e.Call.Args[1]).raw.(string)

					res := &object{false, e.Call.Type()}

					/* file permissions:
					- `6` (owner): read (4) + write (2) = 6
					- `4` (group): read only
					- `4` (others): read only
					*/
					if err := os.WriteFile(path, []byte(content), 0644); err == nil {
						res.raw = true
					}
					return res
				default:
					panic(fmt.Errorf("Unimplemented: fs::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/io" {
				switch e.Call.Name {
				case "print":
					toPrint := vm.eval(&checker.InstanceMethod{
						Subject: e.Call.Args[0],
						Method: &checker.FunctionCall{
							Name: "to_str",
							Args: []checker.Expression{},
						},
					}).raw.(string)

					fmt.Println(toPrint)
					return void
				case "read_line":
					scanner := bufio.NewScanner(os.Stdin)
					scanner.Scan()
					resultType := e.Call.Type().(*checker.Result)
					if err := scanner.Err(); err != nil {
						return makeErr(&object{err.Error(), resultType.Err()}, resultType)
					}
					return makeOk(&object{scanner.Text(), resultType.Val()}, resultType)
				default:
					panic(fmt.Errorf("Unimplemented: io::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/json" {
				switch e.Call.Name {
				case "encode":
					{
						val := vm.eval(e.Call.Args[0])
						resultType := e.Call.Type().(*checker.Result)
						bytes, err := json.Marshal(val.premarshal())
						if err != nil {
							return makeErr(&object{err.Error(), checker.Str}, resultType)
						}
						return makeOk(&object{string(bytes), checker.Str}, resultType)
					}
				case "decode":
					{
						resultType := e.Call.Type().(*checker.Result)
						errorResult := makeErr(&object{"Parsing Error", checker.Str}, resultType)
						result := makeOk(nil, resultType)
						jsonString := vm.eval(e.Call.Args[0]).raw.(string)
						jsonBytes := []byte(jsonString)

						inner := resultType.Val()
						anyType, isAny := inner.(*checker.Any)
						maybeType, isMaybe := inner.(*checker.Maybe)
						// if inner is a generic, reach all the way to the core
						for (isAny && anyType.Actual() != nil) || (isMaybe) {
							if isAny && anyType.Actual() != nil {
								inner = anyType.Actual()
							} else {
								inner = maybeType.Of()
							}
							anyType, isAny = inner.(*checker.Any)
							maybeType, isMaybe = inner.(*checker.Maybe)
						}

						if inner == checker.Str {
							res := &_result{ok: true, raw: &object{jsonString, checker.Str}}
							if isMaybe {
								res.raw._type = maybeType
							}
							result.raw = *res
							return result
						}

						switch subj := inner.(type) {
						case *checker.StructDef:
							{
								_map := make(map[string]any)
								err := json.Unmarshal(jsonBytes, &_map)
								if err != nil {
									result.raw = _result{raw: &object{err.Error(), checker.Str}}
									return result
								}

								fields := make(map[string]*object)
								decoder := json.NewDecoder(strings.NewReader(jsonString))

								if t, err := decoder.Token(); err != nil {
									result.raw = _result{
										raw: &object{
											fmt.Errorf("Error taking opening brace: [%w] %T - %v\n", err, t, t),
											checker.Str,
										},
									}
									return result
								}

								for decoder.More() {
									keyToken, err := decoder.Token()
									if err != nil {
										result.raw = _result{
											raw: &object{
												fmt.Errorf("Error decoding key: [%w] %T - %v\n", err, keyToken, keyToken),
												checker.Str,
											},
										}
										return result
									}
									valToken, err := decoder.Token()
									if err != nil {
										result.raw = _result{
											raw: &object{
												fmt.Errorf("Error decoding value: [%w] %T - %v\n", err, valToken, valToken),
												checker.Str,
											},
										}
										return result
									}
									key := keyToken.(string)

									switch val := valToken.(type) {
									case string:
										valType := subj.Fields[key]
										var decodeAs checker.Type = valType
										// maybe, isMaybe := valType.(*checker.Maybe)
										// if isMaybe {
										// 	decodeAs = maybe
										// } else {
										// 	decodeAs = valType
										// }

										decoded := vm.eval(&checker.PackageFunctionCall{
											Package: "ard/json",
											Call: checker.CreateCall("decode",
												[]checker.Expression{&checker.StrLiteral{Value: val}},
												checker.FunctionDef{
													ReturnType: checker.MakeResult(decodeAs, checker.Str),
												},
											),
										})
										// if err
										if !decoded.raw.(_result).ok {
											return decoded
										}
										// if !isMaybe {
										// 	decoded._type = decodeAs.Of()
										// }
										raw := decoded.raw.(_result).raw
										if maybe, isMaybe := valType.(*checker.Maybe); isMaybe {
											raw._type = maybe
										}
										fields[key] = raw
									case float64:
										if subj.Fields[key] == checker.Float {
											fields[key] = &object{val, checker.Float}
										} else if subj.Fields[key] == checker.Int {
											fields[key] = &object{int(val), checker.Int}
										} else {
											return errorResult
										}
									case bool:
										if subj.Fields[key] != checker.Bool {
											return errorResult
										}
										fields[key] = &object{val, checker.Bool}
									case nil:
										if maybe, isMaybe := subj.Fields[key].(*checker.Maybe); !isMaybe {
											return errorResult
										} else {
											fields[key] = &object{val, maybe}
										}
									case json.Delim:
										if val.String() == "[" {
											listType, ok := subj.Fields[key].(*checker.List)
											if !ok {
												return errorResult
											}
											list := []*object{}
											for decoder.More() {
												var v any
												if err := decoder.Decode(&v); err != nil {
													return errorResult
												}
												obj := enforceSchema(vm, v, listType.Of())
												if obj == nil {
													return errorResult
												}
												list = append(list, obj)
											}
											if t, err := decoder.Token(); err != nil {
												log.Fatal(fmt.Errorf("Error taking closing ]: [%w] %T - %v\n", err, t, t))
												return errorResult
											}

											fields[key] = &object{list, listType}
										} else {
											panic("TODO: handle other json delimiters - " + val.String())
										}
									default:
										panic(fmt.Errorf("unexpected: %v", val))
									}
								}

								for name, fType := range subj.Fields {
									if _, ok := fields[name]; !ok {
										maybe, isMaybe := fType.(*checker.Maybe)
										if !isMaybe {
											return makeErr(&object{"Missing required property: " + name, checker.Str}, resultType)
										}
										fields[name] = &object{nil, maybe}
									}
								}

								// for name, fType := range subj.Fields {
								// 	val := _map[name]
								// 	if f64, ok := val.(float64); ok && fType == (checker.Int) {
								// 		val = int(f64)
								// 	}
								// 	fields[name] = &object{val, fType}
								// }

								result.raw = _result{ok: true, raw: &object{fields, subj}}
								return result
							}
						case *checker.List:
							{
								array := []any{}
								err := json.Unmarshal([]byte(jsonBytes), &array)
								if err != nil {
									result.raw = &object{err.Error(), checker.Str}
									return result
								}

								raw := make([]*object, len(array))
								for i := range array {
									raw[i] = &object{array[i], subj.Of()}
								}

								rawObj := &object{raw, subj}
								result.raw = _result{ok: true, raw: rawObj}
								return result
							}
						default:
							panic(fmt.Errorf("unable to decode into %s", subj))
						}
					}
				default:
					panic(fmt.Errorf("Unimplemented: json::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/maybe" {
				switch e.Call.Name {
				case "none":
					return &object{nil, e.Call.Type()}
				case "some":
					arg := vm.eval(e.Call.Args[0])
					arg._type = e.Call.Type()
					return arg
				default:
					panic(fmt.Errorf("Unimplemented: maybe::%s()", e.Call.Name))
				}
			}

			if e.Package == "ard/http" {
				return evalInHTTP(vm, e.Call)
			}

			if e.Package == "ard/result" {
				return evalInResult(vm, e.Call)
			}

			panic(fmt.Errorf("Unimplemented: %s::%s()", e.Package, e.Call.Name))
		}
	case *checker.ListLiteral:
		{
			raw := make([]*object, len(e.Elements))
			for i, el := range e.Elements {
				raw[i] = vm.eval(el)
			}
			return &object{raw, e.Type()}
		}
	case *checker.MapLiteral:
		{
			raw := make(map[string]*object)
			for i := range e.Keys {
				key := vm.eval(e.Keys[i])
				value := vm.eval(e.Values[i])

				// Create a string representation for the key
				var keyStr string
				switch v := key.raw.(type) {
				case string:
					keyStr = v
				case int:
					keyStr = strconv.Itoa(v)
				case bool:
					keyStr = strconv.FormatBool(v)
				case float64:
					keyStr = strconv.FormatFloat(v, 'g', -1, 64)
				default:
					// For complex types use the pointer address
					keyStr = fmt.Sprintf("%p", key.raw)
				}

				raw[keyStr] = value
			}
			return &object{raw, e.Type()}
		}
	case *checker.OptionMatch:
		{
			subject := vm.eval(e.Subject)
			if subject.raw == nil {
				// None case - evaluate the None block
				res, _ := vm.evalBlock(e.None, nil)
				return res
			} else {
				// Some case - bind the value and evaluate the Some block
				res, _ := vm.evalBlock(e.Some.Body, func() {
					// Bind the pattern name to the value
					subject := &object{subject.raw, subject._type.(*checker.Maybe).Of()}
					vm.scope.add(e.Some.Pattern.Name, subject)
				})
				return res
			}
		}
	case *checker.EnumMatch:
		{
			subject := vm.eval(e.Subject)
			variantIndex := subject.raw.(int8)

			// If there is a catch-all case and we do not have a specific handler for this variant
			if e.CatchAll != nil && (variantIndex >= int8(len(e.Cases)) || e.Cases[variantIndex] == nil) {
				res, _ := vm.evalBlock(e.CatchAll, nil)
				return res
			}

			// Execute the matching case block for this variant
			if variantIndex < int8(len(e.Cases)) && e.Cases[variantIndex] != nil {
				res, _ := vm.evalBlock(e.Cases[variantIndex], nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for enum variant %d", variantIndex))
		}
	case *checker.EnumVariant:
		return &object{e.Variant, e.Type()}
	case *checker.BoolMatch:
		{
			subject := vm.eval(e.Subject)
			value := subject.raw.(bool)

			// Execute the appropriate case based on the boolean value
			if value {
				res, _ := vm.evalBlock(e.True, nil)
				return res
			} else {
				res, _ := vm.evalBlock(e.False, nil)
				return res
			}
		}
	case *checker.UnionMatch:
		{
			subject := vm.eval(e.Subject)

			// Get the concrete type name as a string
			typeName := subject._type.(checker.Type).String()

			// If we have a case for this specific type
			if block, ok := e.TypeCases[typeName]; ok {
				res, _ := vm.evalBlock(block, func() {
					// Bind the pattern variable 'it' to the value
					vm.scope.add("it", subject)
				})
				return res
			}

			// If we have a catch-all case
			if e.CatchAll != nil {
				res, _ := vm.evalBlock(e.CatchAll, nil)
				return res
			}

			// This should never happen if the type checker is working correctly
			// because it ensures the match is exhaustive
			panic(fmt.Errorf("No matching case for union type %s", typeName))
		}
	case *checker.StructInstance:
		{
			strct := e.Type().(*checker.StructDef)
			raw := map[string]*object{}
			for name, ftype := range strct.Fields {
				val, ok := e.Fields[name]
				if ok {
					raw[name] = vm.eval(val)
				} else {
					// assume it's a Maybe<T> if the checker allowed it
					raw[name] = &object{nil, ftype}
				}
			}
			return &object{raw, e.Type()}
		}
	case *checker.PackageStructInstance:
		{
			if e.Package == "ard/http" {
				return vm.eval(e.Property)
			}
			panic(fmt.Errorf("Unimplemented in package: %s", e.Package))
		}
	case *checker.ResultMatch:
		{
			subj := vm.eval(e.Subject)
			raw := subj.raw.(_result)

			if raw.ok {
				res, _ := vm.evalBlock(e.Ok.Body, func() {
					vm.scope.add(e.Ok.Pattern.Name, raw.raw)
				})
				return res
			}
			res, _ := vm.evalBlock(e.Err.Body, func() {
				vm.scope.add(e.Err.Pattern.Name, raw.raw)
			})
			return res
		}
	case *checker.TryOp:
		{
			subj := vm.eval(e.Expr())
			switch _type := subj._type.(type) {
			case *checker.Result:
				raw := subj.raw.(_result)
				if !raw.ok {
					vm.scope.broken = true
					return subj
				}

				return raw.raw
			default:
				panic(fmt.Errorf("Cannot match on %s", _type))
			}
		}
	default:
		panic(fmt.Errorf("Unimplemented expression: %T", e))
	}
}

func (vm *VM) evalBlock(block *checker.Block, init func()) (*object, bool) {
	vm.pushScope()
	defer vm.popScope()

	if init != nil {
		init()
	}

	res := void
	for i := range block.Stmts {
		stmt := block.Stmts[i]
		r := vm.do(stmt)
		if vm.scope.broken {
			return r, true
		}
		res = r
	}

	return res, false
}

func (vm *VM) evalStrProperty(subj *object, name string) *object {
	switch name {
	case "size":
		return &object{len(subj.raw.(string)), checker.Int}
	default:
		return void
	}
}

func (vm *VM) evalInstanceMethod(self *object, e *checker.InstanceMethod) *object {
	if self._type == checker.Str {
		return vm.evalStrMethod(self, e.Method)
	}
	if self._type == checker.Int {
		return vm.evalIntMethod(self, e)
	}
	if self._type == checker.Float {
		return vm.evalFloatMethod(self, e.Method)
	}
	if self._type == checker.Bool {
		return vm.evalBoolMethod(self, e)
	}
	if _, ok := self._type.(*checker.List); ok {
		return vm.evalListMethod(self, e)
	}
	if _, ok := self._type.(*checker.Map); ok {
		return vm.evalMapMethod(self, e)
	}
	if _, ok := self._type.(*checker.Maybe); ok {
		return vm.evalMaybeMethod(self, e)
	}
	if _, ok := self._type.(*checker.StructDef); ok {
		return vm.evalStructMethod(self, e.Method)
	}
	if _, ok := self._type.(*checker.Result); ok {
		return vm.evalResultMethod(self, e.Method)
	}
	if any, ok := self._type.(*checker.Any); ok {
		self._type = any.Actual()
		res := vm.evalInstanceMethod(self, e)
		self._type = any
		return res
	}

	panic(fmt.Errorf("Unimplemented: %s.%s() on %T", self._type, e.Method.Name, self._type))
}

func (vm *VM) evalStrMethod(subj *object, m *checker.FunctionCall) *object {
	raw := subj.raw.(string)
	switch m.Name {
	case "size":
		return &object{len(raw), m.Type()}
	case "is_empty":
		return &object{len(raw) == 0, m.Type()}
	case "contains":
		return &object{strings.Contains(raw, vm.eval(m.Args[0]).raw.(string)), m.Type()}
	case "split":
		sep := vm.eval(m.Args[0]).raw.(string)
		_list := strings.Split(raw, sep)
		list := make([]*object, len(_list))

		for i, str := range _list {
			list[i] = &object{str, checker.Str}
		}

		return &object{list, m.Type()}
	case "to_str":
		return subj
	case "trim":
		return &object{strings.Trim(raw, " "), m.Type()}
	default:
		panic(fmt.Errorf(`Undefined method: "%s".%s()`, raw, m.Name))
	}
}

func (vm *VM) evalIntMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.Itoa(subj.raw.(int)), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalFloatMethod(subj *object, m *checker.FunctionCall) *object {
	switch m.Name {
	case "to_str":
		return &object{strconv.FormatFloat(subj.raw.(float64), 'f', 2, 64), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalBoolMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "to_str":
		return &object{strconv.FormatBool(subj.raw.(bool)), checker.Str}
	default:
		return void
	}
}

func (vm *VM) evalListMethod(self *object, m *checker.InstanceMethod) *object {
	raw := self.raw.([]*object)
	switch m.Method.Name {
	case "at":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		if index >= len(raw) {
			panic(fmt.Errorf("Index out of range (%d) on list of length %d", index, len(raw)))
		}
		return &object{raw[index].raw, m.Type()}
	case "push":
		self.raw = append(raw, vm.eval(m.Method.Args[0]))
		return self
	case "set":
		index := vm.eval(m.Method.Args[0]).raw.(int)
		value := vm.eval(m.Method.Args[1])
		result := &object{false, checker.Bool}
		if index <= len(raw) {
			raw[index] = value
			result.raw = true
		}
		return result
	case "size":
		return &object{len(raw), checker.Int}
	case "swap":
		l := vm.eval(m.Method.Args[0]).raw.(int)
		r := vm.eval(m.Method.Args[1]).raw.(int)
		_l, _r := raw[l], raw[r]
		raw[l] = _r
		raw[r] = _l
		return void
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", self._type, m.Method.Name))
	}
}

func (vm *VM) evalMapMethod(subj *object, m *checker.InstanceMethod) *object {
	raw := subj.raw.(map[string]*object)
	switch m.Method.Name {
	case "keys":
		keys := make([]*object, len(raw))
		i := 0
		for k := range raw {
			keys[i] = &object{k, checker.Str}
			i += 1
		}
		return &object{keys, m.Type()}
	case "size":
		return &object{len(raw), checker.Int}
	case "get":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Try to find the key
		value, found := raw[keyStr]
		if !found {
			// Return nil for the maybe type
			return &object{nil, m.Type()}
		}
		return &object{value.raw, m.Type()}
	case "set":
		keyArg := vm.eval(m.Method.Args[0])
		valueArg := vm.eval(m.Method.Args[1])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Add or update the entry
		raw[keyStr] = valueArg
		// Return success
		return &object{true, checker.Bool}
	case "drop":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Remove the entry
		delete(raw, keyStr)
		return void
	case "has":
		keyArg := vm.eval(m.Method.Args[0])

		// Convert key to string
		var keyStr string
		switch v := keyArg.raw.(type) {
		case string:
			keyStr = v
		case int:
			keyStr = strconv.Itoa(v)
		case bool:
			keyStr = strconv.FormatBool(v)
		case float64:
			keyStr = strconv.FormatFloat(v, 'g', -1, 64)
		default:
			keyStr = fmt.Sprintf("%p", keyArg.raw)
		}

		// Check if the key exists
		_, found := raw[keyStr]
		return &object{found, checker.Bool}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalMaybeMethod(subj *object, m *checker.InstanceMethod) *object {
	switch m.Method.Name {
	case "is_none":
		return &object{subj.raw == nil, m.Type()}
	case "is_some":
		return &object{subj.raw != nil, m.Type()}
	case "or":
		if subj.raw == nil {
			return vm.eval(m.Method.Args[0])
		}
		return &object{subj.raw, m.Type()}
	default:
		panic(fmt.Errorf("Unimplemented: %s.%s()", subj._type, m.Method.Name))
	}
}

func (vm *VM) evalStructMethod(subj *object, call *checker.FunctionCall) *object {
	istruct := subj._type.(*checker.StructDef)

	// Special handling for HTTP Response methods
	if istruct == checker.HttpResponseDef {
		return vm.evalHttpResponseMethod(subj, call)
	}

	sig, ok := istruct.Fields[call.Name]
	if !ok {
		panic(fmt.Errorf("Undefined: %s.%s", istruct.Name, call.Name))
	}

	fnDef, ok := sig.(*checker.FunctionDef)
	if !ok {
		panic(fmt.Errorf("Not a function: %s.%s", istruct.Name, call.Name))
	}

	fn := func(args ...*object) *object {
		res, _ := vm.evalBlock(fnDef.Body, func() {
			vm.scope.add(fnDef.SelfName, subj)
			for i := range args {
				vm.scope.add(fnDef.Parameters[i].Name, args[i])
			}
		})
		return res
	}

	args := make([]*object, len(call.Args))
	for i := range call.Args {
		args[i] = vm.eval(call.Args[i])
	}

	return fn(args...)
}

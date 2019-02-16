# python supervisord multicall 分析

## 1 请求参数
---

## 1.1 请求参数为字符串数组

```Python
def test_multicall_performs_noncallback_functions_serially(self):
	        class DummyNamespace(object):
	            def say(self, name):
	                """ @param string name Process name"""
	                return name
	        ns1 = DummyNamespace()
	        inst = self._makeOne([('ns1', ns1)])
	        calls = [
	            {'methodName': 'ns1.say', 'params': ['Alvin']},
	            {'methodName': 'ns1.say', 'params': ['Simon']},
	            {'methodName': 'ns1.say', 'params': ['Theodore']}
	        ]
	        results = inst.multicall(calls)
	        self.assertEqual(results, ['Alvin', 'Simon', 'Theodore'])
```


## 1.2 请求参数为空

```Python
    def test_multicall_catches_callback_exceptions(self):
	        ns1 = DummyNamespace()
	        inst = self._makeOne([('ns1', ns1)])
	        calls = [{'methodName': 'ns1.bad_name'}, {'methodName': 'ns1.os_error'}]
	        callback = inst.multicall(calls)
	        results = NOT_DONE_YET
	        while results is NOT_DONE_YET:
	            results = callback()

	        bad_name = {'faultCode': Faults.BAD_NAME,
	                    'faultString': 'BAD_NAME: foo'}
	        os_error = {'faultCode': Faults.FAILED,
	                    'faultString': "FAILED: %s:2" % OSError}
	        self.assertEqual(results, [bad_name, os_error])
```

## 1.3 请求参数为单个 json 对象

```Python
    def test_multicall_performs_callback_functions_serially(self):
	        ns1 = DummyNamespace()
	        inst = self._makeOne([('ns1', ns1)])
	        calls = [{'methodName': 'ns1.stopProcess',
	                  'params': {'name': 'foo'}},
	                 {'methodName': 'ns1.startProcess',
	                  'params': {'name': 'foo'}}]
	        callback = inst.multicall(calls)
	        results = NOT_DONE_YET
	        while results is NOT_DONE_YET:
	            results = callback()
	        self.assertEqual(results, ['stop result', 'start result'])
```

## 1.4 结论

目前go-supervisord只处理请求参数为空和请求参数为字符串数组的 case，整体请求参数定义如下：

```Go
type ApiMethod struct {
	MethodName string   `xml:"methodName" json:"methodName"`
	Params     []string `xml:"params" json:"params"`
}

type MulticallArgs struct {
	Methods []ApiMethod
}
```

## 2 响应参数
---

System.Multicall 的响应结果分为成功响应和失败响应两种请情况，成功响应时返回一个 struct，失败的时候，需要返回如下失败内容：

```Go
type StateInfo struct {
  Statecode int    `xml:"statecode"`
  Statename string `xml:"statename"`
}
```

恰好这个结构体 github.com/alexstocks/gorilla-xmlrpc 中有定义，直接使用即可。

响应结果定义如下：

```Go
type MulticallResults struct {
	Results []interface{}
}
```

并自定义了改结构体的 XML 序列化函数。

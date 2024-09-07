def handler(params, context):
    try:
        arr = params["array"]
        
        n = len(arr)
        for i in range(n):
            for j in range(0, n-i-1):
                if arr[j] > arr[j+1]:
                    arr[j], arr[j+1] = arr[j+1], arr[j]
        
        return {"SortedArray": arr}
    except:
        return {}

def max_subarray_sum(arr):
    max_ending_here = max_so_far = arr[0]
    for x in arr[1:]:
        max_ending_here = max(x, max_ending_here + x)
        max_so_far = max(max_so_far, max_ending_here)
    return max_so_far

def handler(params, context):
    try:
        arr = params["array"]
        result = max_subarray_sum(arr)
        return {"Result": result}
    except:
        return {}

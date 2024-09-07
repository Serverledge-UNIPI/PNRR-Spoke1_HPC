def count_inversions(arr):
    def merge_count_split_inv(arr, temp_arr, left, right):
        if left >= right:
            return 0
        mid = (left + right) // 2
        inv_count = merge_count_split_inv(arr, temp_arr, left, mid)
        inv_count += merge_count_split_inv(arr, temp_arr, mid + 1, right)
        inv_count += merge_and_count(arr, temp_arr, left, mid, right)
        return inv_count

    def merge_and_count(arr, temp_arr, left, mid, right):
        i, j, k = left, mid + 1, left
        inv_count = 0
        while i <= mid and j <= right:
            if arr[i] <= arr[j]:
                temp_arr[k] = arr[i]
                i += 1
            else:
                temp_arr[k] = arr[j]
                inv_count += (mid-i + 1)
                j += 1
            k += 1
        while i <= mid:
            temp_arr[k] = arr[i]
            i += 1
            k += 1
        while j <= right:
            temp_arr[k] = arr[j]
            j += 1
            k += 1
        arr[left:right + 1] = temp_arr[left:right + 1]
        return inv_count

    temp_arr = [0] * len(arr)
    return merge_count_split_inv(arr, temp_arr, 0, len(arr) - 1)

def handler(params, context):
    try:
        arr = params["array"]
        result = count_inversions(arr)
        return { "Result": result }
    except:
        return {}
